package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/hkdf"
)

// TranscriptionRequest representa a estrutura da requisição recebida no endpoint /transcription
type TranscriptionRequest struct {
	AudioBase64    string `json:"audio_base64,omitempty"`    // Áudio em Base64 (para media_type "audio")
	VideoBase64    string `json:"video_base64,omitempty"`    // Vídeo em Base64 (para media_type "video")
	ImageBase64    string `json:"image_base64,omitempty"`    // Imagem em Base64 (para media_type "image")
	EncryptedAudio string `json:"encrypted_audio,omitempty"` // NOVO: Áudio criptografado do WhatsApp (.enc)
	MediaKey       string `json:"media_key,omitempty"`       // NOVO: Chave de mídia do WhatsApp
	MediaType      string `json:"media_type"`                // Tipo de mídia: "audio", "video" ou "image"
}

// TranscriptionResponse representa a estrutura da resposta retornada pelo endpoint /transcription
type TranscriptionResponse struct {
	Transcription       string `json:"transcription"`
	AudioResponseBase64 string `json:"audio_response_base64,omitempty"` // Áudio de resposta gerado em Base64
	Language            string `json:"language"`                        // Idioma detectado (ex.: "pt", "en")
	Error               string `json:"error,omitempty"`                 // Mensagem de erro, se houver
}

// --- CONFIGURAÇÃO DE CRIPTOGRAFIA DO WHATSAPP (NOVO) ---

var waInfoStrings = map[string]string{
	"audio":    "WhatsApp Audio Keys",
	"video":    "WhatsApp Video Keys",
	"image":    "WhatsApp Image Keys",
	"document": "WhatsApp Document Keys",
}

// pkcs7Unpadding remove o preenchimento padrão do AES
func pkcs7Unpadding(src []byte) ([]byte, error) {
	length := len(src)
	if length == 0 {
		return nil, errors.New("input inválido para unpadding")
	}
	padding := int(src[length-1])
	if padding > length || padding == 0 {
		return nil, errors.New("padding inválido ou corrompido")
	}
	return src[:length-padding], nil
}

// decryptWhatsAppMedia implementa a descriptografia do Signal/WhatsApp
func decryptWhatsAppMedia(encAudioBase64, mediaKeyBase64, mediaType string) ([]byte, error) {
    // 1. Decodificar Base64
    encData, err := base64.StdEncoding.DecodeString(encAudioBase64)
    if err != nil {
        return nil, fmt.Errorf("erro ao decodificar encrypted_audio: %v", err)
    }

    mediaKey, err := base64.StdEncoding.DecodeString(mediaKeyBase64)
    if err != nil {
        return nil, fmt.Errorf("erro ao decodificar media_key: %v", err)
    }

    // 2. Remover os 10 bytes de MAC (obrigatório no formato do WhatsApp)
    if len(encData) <= 10 {
        return nil, errors.New("arquivo criptografado muito curto")
    }
    cipherText := encData[:len(encData)-10]   // ← CORREÇÃO PRINCIPAL

    // 3. Definir a Info String correta
    infoString, ok := waInfoStrings[mediaType]
    if !ok {
        infoString = "WhatsApp Audio Keys" // Fallback seguro
    }

    // 4. Derivação de Chaves (HKDF-SHA256)
    salt := make([]byte, 32)
    hkdfReader := hkdf.New(sha256.New, mediaKey, salt, []byte(infoString))

    keyMaterial := make([]byte, 112)
    if _, err := io.ReadFull(hkdfReader, keyMaterial); err != nil {
        return nil, fmt.Errorf("erro na derivação HKDF: %v", err)
    }

    // 5. Fatiar chaves
    iv := keyMaterial[:16]
    cipherKey := keyMaterial[16:48]

    // 6. Descriptografar (AES-256-CBC)
    block, err := aes.NewCipher(cipherKey)
    if err != nil {
        return nil, fmt.Errorf("erro ao criar cifra AES: %v", err)
    }

    if len(cipherText)%aes.BlockSize != 0 {
        return nil, fmt.Errorf("tamanho do ciphertext inválido após remover MAC: %d bytes", len(cipherText))
    }

    mode := cipher.NewCBCDecrypter(block, iv)
    decrypted := make([]byte, len(cipherText))
    mode.CryptBlocks(decrypted, cipherText)

    // 7. Remover Padding PKCS7
    return pkcs7Unpadding(decrypted)
}

// --- FIM DA CONFIGURAÇÃO DE CRIPTOGRAFIA ---

// whisperLangFullToShort mapeia o nome completo do idioma que o
// whisper-server devolve (response_format=verbose_json, ex.: "portuguese")
// pro codigo curto de 2 letras que o resto do codigo espera (ex.: "pt",
// igual ao que o CLI com --output-json devolvia em result.language).
// Tabela extraida direto de src/whisper.cpp (g_lang) do proprio whisper.cpp,
// pra bater exatamente com o que o binario ja usa.
var whisperLangFullToShort = map[string]string{
	"english": "en", "chinese": "zh", "german": "de", "spanish": "es",
	"russian": "ru", "korean": "ko", "french": "fr", "japanese": "ja",
	"portuguese": "pt", "turkish": "tr", "polish": "pl", "catalan": "ca",
	"dutch": "nl", "arabic": "ar", "swedish": "sv", "italian": "it",
	"indonesian": "id", "hindi": "hi", "finnish": "fi", "vietnamese": "vi",
	"hebrew": "he", "ukrainian": "uk", "greek": "el", "malay": "ms",
	"czech": "cs", "romanian": "ro", "danish": "da", "hungarian": "hu",
	"tamil": "ta", "norwegian": "no", "thai": "th", "urdu": "ur",
	"croatian": "hr", "bulgarian": "bg", "lithuanian": "lt", "latin": "la",
	"maori": "mi", "malayalam": "ml", "welsh": "cy", "slovak": "sk",
	"telugu": "te", "persian": "fa", "latvian": "lv", "bengali": "bn",
	"serbian": "sr", "azerbaijani": "az", "slovenian": "sl", "kannada": "kn",
	"estonian": "et", "macedonian": "mk", "breton": "br", "basque": "eu",
	"icelandic": "is", "armenian": "hy", "nepali": "ne", "mongolian": "mn",
	"bosnian": "bs", "kazakh": "kk", "albanian": "sq", "swahili": "sw",
	"galician": "gl", "marathi": "mr", "punjabi": "pa", "sinhala": "si",
	"khmer": "km", "shona": "sn", "yoruba": "yo", "somali": "so",
	"afrikaans": "af", "occitan": "oc", "georgian": "ka", "belarusian": "be",
	"tajik": "tg", "sindhi": "sd", "gujarati": "gu", "amharic": "am",
	"yiddish": "yi", "lao": "lo", "uzbek": "uz", "faroese": "fo",
	"haitian creole": "ht", "pashto": "ps", "turkmen": "tk", "nynorsk": "nn",
	"maltese": "mt", "sanskrit": "sa", "luxembourgish": "lb", "myanmar": "my",
	"tibetan": "bo", "tagalog": "tl", "malagasy": "mg", "assamese": "as",
	"tatar": "tt", "hawaiian": "haw", "lingala": "ln", "hausa": "ha",
	"bashkir": "ba", "javanese": "jw", "sundanese": "su", "cantonese": "yue",
}

// whisperServerURL retorna a URL do endpoint /inference do whisper-server
// local, configuravel via WHISPER_SERVER_URL (mesmo padrao de
// WHISPER_MODEL_PATH antes). Default aponta pro processo pm2
// "whisper-server" nessa mesma VPS (modelo carregado uma unica vez no boot
// dele, sempre residente em memoria).
func whisperServerURL() string {
	if url := os.Getenv("WHISPER_SERVER_URL"); url != "" {
		return url
	}
	return "http://127.0.0.1:8090/inference"
}

// transcribeWithWhisperServer chama o whisper-server (processo persistente,
// modelo residente em memoria -- ver whisperServerURL) em vez de invocar o
// binario CLI do whisper.cpp como subprocesso novo a cada chamada. O
// reload/init do modelo a cada exec.Command era a maior parte do tempo
// gasto (medido: ~26-65s via CLI virou ~3-22s via server dependendo do
// modelo, sem mudar a precisao -- mesmo texto, so' mais rapido).
func transcribeWithWhisperServer(wavPath string) (string, string, error) {
	file, err := os.Open(wavPath)
	if err != nil {
		return "", "", fmt.Errorf("erro ao abrir wav pra envio: %v", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(wavPath))
	if err != nil {
		return "", "", fmt.Errorf("erro ao montar multipart: %v", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", "", fmt.Errorf("erro ao copiar wav pro multipart: %v", err)
	}
	writer.WriteField("response_format", "verbose_json")
	writer.WriteField("language", "auto")
	if err := writer.Close(); err != nil {
		return "", "", fmt.Errorf("erro ao finalizar multipart: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, whisperServerURL(), &body)
	if err != nil {
		return "", "", fmt.Errorf("erro ao montar requisição pro whisper-server: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("erro ao chamar whisper-server: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("erro ao ler resposta do whisper-server: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("whisper-server respondeu %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Text     string `json:"text"`
		Language string `json:"language"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", fmt.Errorf("erro ao parsear resposta do whisper-server: %v (corpo: %s)", err, string(respBody))
	}

	transcription := strings.TrimSpace(result.Text)
	if transcription == "" {
		return "", "", errors.New("nenhuma transcrição encontrada")
	}

	language := whisperLangFullToShort[strings.ToLower(result.Language)]
	if language == "" {
		language = result.Language
	}

	return transcription, language, nil
}

// transcriptionHandler é o handler para o endpoint /transcription
func transcriptionHandler(w http.ResponseWriter, r *http.Request) {
	// Verifica se o método é POST
	if r.Method!= http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	// Aumentar o limite de leitura do corpo para suportar arquivos maiores (ex: 50MB)
	r.Body = http.MaxBytesReader(w, r.Body, 50*1024*1024)

	// Decodifica a requisição JSON
	var req TranscriptionRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err!= nil {
		resp := TranscriptionResponse{Error: "Erro ao decodificar o corpo da requisição: " + err.Error()}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Validação do campo media_type
	if req.MediaType == "" {
		resp := TranscriptionResponse{Error: "media_type é obrigatório (audio, video, image)"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Variável para armazenar os bytes finais do arquivo (já decodificados/descriptografados)
	var data []byte

	// --- LÓGICA DE SELEÇÃO DE DADOS (ATUALIZADA) ---
	// Verifica primeiro se é um áudio criptografado do WhatsApp
	if req.EncryptedAudio!= "" && req.MediaKey!= "" {
		log.Println("Processando áudio criptografado do WhatsApp...")
		data, err = decryptWhatsAppMedia(req.EncryptedAudio, req.MediaKey, req.MediaType)
		if err!= nil {
			resp := TranscriptionResponse{Error: "Falha na descriptografia: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		log.Println("Áudio descriptografado com sucesso.")
	} else {
		// Se não for criptografado, segue a lógica original de Base64
		var inputData string
		switch req.MediaType {
		case "audio":
			if req.AudioBase64 == "" {
				resp := TranscriptionResponse{Error: "audio_base64 (ou encrypted_audio) é obrigatório para media_type audio"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			// Verifica se o valor de AudioBase64 é um JSON aninhado
			var nestedData struct {
				AudioBase64 string `json:"audio_base64"`
			}
			// Tenta fazer o unmarshal caso seja um JSON stringificado
			if json.Unmarshal([]byte(req.AudioBase64), &nestedData) == nil && nestedData.AudioBase64!= "" {
				inputData = nestedData.AudioBase64
			} else {
				inputData = req.AudioBase64
			}
		case "video":
			if req.VideoBase64 == "" {
				resp := TranscriptionResponse{Error: "video_base64 é obrigatório para media_type video"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			inputData = req.VideoBase64
		case "image":
			if req.ImageBase64 == "" {
				resp := TranscriptionResponse{Error: "image_base64 é obrigatório para media_type image"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			inputData = req.ImageBase64
		default:
			resp := TranscriptionResponse{Error: "media_type inválido (deve ser audio, video ou image)"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Valida se os dados base64 não estão vazios
		if inputData == "" {
			resp := TranscriptionResponse{Error: "Dados base64 não podem estar vazios"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Decodifica os dados Base64
		data, err = base64.StdEncoding.DecodeString(inputData)
		if err!= nil {
			resp := TranscriptionResponse{Error: "Erro ao decodificar dados base64: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
	}

	// Verifica se os dados (seja via descriptografia ou base64) estão vazios
	if len(data) == 0 {
		resp := TranscriptionResponse{Error: "Dados finais do arquivo estão vazios"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}
	log.Printf("Tamanho dos dados a processar: %d bytes", len(data))

	// Cria um diretório temporário para arquivos
	tempDir, err := os.MkdirTemp("", "temp-")
	if err!= nil {
		resp := TranscriptionResponse{Error: "Erro ao criar diretório temporário: " + err.Error()}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}
	log.Printf("Diretório temporário criado: %s", tempDir)
	defer os.RemoveAll(tempDir)

	// Salva o arquivo de entrada
	var inputFile, outputFile string
	switch req.MediaType {
	case "audio":
		// Se veio descriptografado do WhatsApp, geralmente é OGG
		inputFile = filepath.Join(tempDir, "input.ogg") 
		outputFile = filepath.Join(tempDir, "output.wav")
	case "video":
		inputFile = filepath.Join(tempDir, "input.mp4")
		outputFile = filepath.Join(tempDir, "audio.wav")
	case "image":
		inputFile = filepath.Join(tempDir, "input.png")
	}

	err = ioutil.WriteFile(inputFile, data, 0644)
	if err!= nil {
		resp := TranscriptionResponse{Error: "Erro ao salvar arquivo de entrada: " + err.Error()}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}
	log.Printf("Arquivo de entrada salvo: %s", inputFile)

	// Processa o arquivo com base no media_type
	var transcription string
	var language string
	
	switch req.MediaType {
	case "audio":
		// Converte o arquivo de áudio para WAV
		start := time.Now()
		// Nota: Mantido o filtro afftdn conforme seu código original
		ffmpegArgs :=[]string{"-i", inputFile, "-ar", "16000", "-ac", "1", "-vn", "-map", "0:a", "-af", "afftdn", "-y", outputFile}
		cmd := exec.Command("ffmpeg", ffmpegArgs...)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err = cmd.Run()
		ffmpegDuration := time.Since(start)
		log.Printf("Tempo de conversão com ffmpeg: %v", ffmpegDuration)
		if err!= nil {
			resp := TranscriptionResponse{Error: "Erro ao converter áudio para WAV: " + err.Error() + " (stderr: " + stderr.String() + ")"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Verifica se o arquivo WAV foi gerado e não está vazio
		fileInfo, err := os.Stat(outputFile)
		if err!= nil {
			resp := TranscriptionResponse{Error: "Arquivo WAV não encontrado: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if fileInfo.Size() == 0 {
			resp := TranscriptionResponse{Error: "Arquivo WAV está vazio"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		log.Printf("Arquivo WAV gerado: %s (%d bytes)", outputFile, fileInfo.Size())

		// Obtém a duração do áudio com ffprobe
		cmd = exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", outputFile)
		stderr.Reset()
		cmd.Stderr = &stderr
		ffprobeOutput, err := cmd.Output()
		if err!= nil {
			resp := TranscriptionResponse{Error: "Erro ao obter duração do áudio: " + err.Error() + " (stderr: " + stderr.String() + ")"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		durationStr := strings.TrimSpace(string(ffprobeOutput))
		if durationStr == "" {
			resp := TranscriptionResponse{Error: "Duração do áudio não retornada pelo ffprobe (stderr: " + stderr.String() + ")"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		duration, err := strconv.ParseFloat(durationStr, 64)
		if err!= nil {
			resp := TranscriptionResponse{Error: "Erro ao parsear duração do áudio: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		log.Printf("Duração do áudio: %.2f segundos", duration)

		// Limita a duração a 120 segundos
		if duration > 120 {
			log.Printf("Áudio excede 120 segundos (%.2f segundos), cortando para 120 segundos", duration)
			tempOutput := filepath.Join(tempDir, "trimmed.wav")
			cmd = exec.Command("ffmpeg", "-i", outputFile, "-t", "120", "-y", tempOutput)
			stderr.Reset()
			cmd.Stderr = &stderr
			err = cmd.Run()
			if err!= nil {
				resp := TranscriptionResponse{Error: "Erro ao cortar áudio: " + err.Error() + " (stderr: " + stderr.String() + ")"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			outputFile = tempOutput
		}

		// Transcreve com Whisper (whisper-server, modelo residente em memoria)
		start = time.Now()
		transcription, language, err = transcribeWithWhisperServer(outputFile)
		whisperDuration := time.Since(start)
		log.Printf("Tempo de transcrição com Whisper: %v", whisperDuration)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao transcrever áudio: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

	case "video":
		// Extrai o áudio do vídeo e converte para WAV
		start := time.Now()
		ffmpegArgs := []string{"-i", inputFile, "-ar", "16000", "-ac", "1", "-vn", "-map", "0:a", "-af", "afftdn", "-y", outputFile}   // ← adicionar []string
		cmd := exec.Command("ffmpeg", ffmpegArgs...)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err = cmd.Run()
		ffmpegDuration := time.Since(start)
		log.Printf("Tempo de extração de áudio com ffmpeg: %v", ffmpegDuration)
		if err!= nil {
			resp := TranscriptionResponse{Error: "Erro ao extrair áudio do vídeo: " + err.Error() + " (stderr: " + stderr.String() + ")"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Verifica se o arquivo WAV foi gerado e não está vazio
		fileInfo, err := os.Stat(outputFile)
		if err!= nil {
			resp := TranscriptionResponse{Error: "Arquivo WAV não encontrado: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if fileInfo.Size() == 0 {
			resp := TranscriptionResponse{Error: "Arquivo WAV está vazio"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		log.Printf("Arquivo WAV gerado: %s (%d bytes)", outputFile, fileInfo.Size())

		// Obtém a duração do áudio com ffprobe
		cmd = exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", outputFile)
		stderr.Reset()
		cmd.Stderr = &stderr
		ffprobeOutput, err := cmd.Output()
		if err!= nil {
			resp := TranscriptionResponse{Error: "Erro ao obter duração do áudio do vídeo: " + err.Error() + " (stderr: " + stderr.String() + ")"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		durationStr := strings.TrimSpace(string(ffprobeOutput))
		if durationStr == "" {
			resp := TranscriptionResponse{Error: "Duração do áudio do vídeo não retornada pelo ffprobe (stderr: " + stderr.String() + ")"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		duration, err := strconv.ParseFloat(durationStr, 64)
		if err!= nil {
			resp := TranscriptionResponse{Error: "Erro ao parsear duração do áudio do vídeo: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		log.Printf("Duração do áudio do vídeo: %.2f segundos", duration)

		// Limita a duração a 120 segundos
		if duration > 120 {
			log.Printf("Áudio do vídeo excede 120 segundos (%.2f segundos), cortando para 120 segundos", duration)
			tempOutput := filepath.Join(tempDir, "trimmed.wav")
			cmd = exec.Command("ffmpeg", "-i", outputFile, "-t", "120", "-y", tempOutput)
			stderr.Reset()
			cmd.Stderr = &stderr
			err = cmd.Run()
			if err!= nil {
				resp := TranscriptionResponse{Error: "Erro ao cortar áudio do vídeo: " + err.Error() + " (stderr: " + stderr.String() + ")"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			outputFile = tempOutput
		}

		// Transcreve com Whisper (whisper-server, modelo residente em memoria)
		start = time.Now()
		transcription, language, err = transcribeWithWhisperServer(outputFile)
		whisperDuration := time.Since(start)
		log.Printf("Tempo de transcrição com Whisper: %v", whisperDuration)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao transcrever áudio do vídeo: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

	case "image":
		// Realiza OCR com Tesseract
		cmd := exec.Command("tesseract", inputFile, "stdout", "-l", "por")
		output, err := cmd.CombinedOutput()
		if err!= nil {
			resp := TranscriptionResponse{Error: "Erro ao realizar OCR: " + string(output)}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		transcription = strings.TrimSpace(string(output))
		language = "pt" // Para imagens, assumimos português por padrão
	}

	// Seleciona o modelo de voz com base no idioma
	var piperModel string
	if language == "pt" {
		piperModel = "pt_BR-faber-medium"
	} else {
		piperModel = "en_US-lessac-medium"
	}

	// Gera áudio de resposta com piper-tts (via Python)
	audioFile := filepath.Join(tempDir, "response.wav")
	responseText := "Transcrição: " + transcription
	piperCmd := exec.Command("/www/wwwroot/dialogix/transcription-api/.venv/bin/piper", "--model", piperModel, "--output_file", audioFile)
	piperCmd.Stdin = strings.NewReader(responseText)
	output, err := piperCmd.CombinedOutput()
	if err!= nil {
		resp := TranscriptionResponse{Error: fmt.Sprintf("Erro ao gerar áudio de resposta: %v - Output: %s - Command: %s", err, string(output), piperCmd.String())}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Lê o áudio gerado e converte para Base64
	audioData, err := ioutil.ReadFile(audioFile)
	if err!= nil {
		resp := TranscriptionResponse{Error: "Erro ao ler áudio de resposta: " + err.Error()}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}
	audioResponseBase64 := base64.StdEncoding.EncodeToString(audioData)

	// Retorna a resposta
	resp := TranscriptionResponse{
		Transcription:       transcription,
		AudioResponseBase64: audioResponseBase64,
		Language:            language,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Função principal
func main() {
	// Registra os handlers
	http.HandleFunc("/transcription", transcriptionHandler)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "API de transcrição funcionando")
	})

	// Inicia o servidor na porta 3200
	log.Println("API rodando na porta 3200")
	err := http.ListenAndServe(":3200", nil)
	if err!= nil {
		log.Fatal("Erro ao iniciar o servidor:", err)
	}
}