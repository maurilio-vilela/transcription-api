package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// TranscriptionRequest representa a estrutura da requisição recebida no endpoint /transcription
type TranscriptionRequest struct {
	AudioBase64 string `json:"audio_base64,omitempty"` // Áudio em Base64 (para media_type "audio")
	VideoBase64 string `json:"video_base64,omitempty"` // Vídeo em Base64 (para media_type "video")
	ImageBase64 string `json:"image_base64,omitempty"` // Imagem em Base64 (para media_type "image")
	MediaType   string `json:"media_type"`             // Tipo de mídia: "audio", "video" ou "image"
}

// TranscriptionResponse representa a estrutura da resposta retornada pelo endpoint /transcription
type TranscriptionResponse struct {
	Transcription       string `json:"transcription"`           // Texto transcrito ou extraído (OCR)
	AudioResponseBase64 string `json:"audio_response_base64,omitempty"` // Áudio de resposta gerado em Base64
	Language            string `json:"language"`                // Idioma detectado (ex.: "pt", "en")
	Error               string `json:"error,omitempty"`         // Mensagem de erro, se houver
}

// transcriptionHandler é o handler para o endpoint /transcription
func transcriptionHandler(w http.ResponseWriter, r *http.Request) {
	// Verifica se o método é POST
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	// Decodifica a requisição JSON
	var req TranscriptionRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Erro ao decodificar o corpo da requisição", http.StatusBadRequest)
		return
	}

	// Validação do campo media_type
	if req.MediaType == "" {
		resp := TranscriptionResponse{Error: "media_type é obrigatório (audio, video, image)"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Valida e seleciona os dados de entrada com base no media_type
	var inputData string
	switch req.MediaType {
	case "audio":
		if req.AudioBase64 == "" {
			resp := TranscriptionResponse{Error: "audio_base64 é obrigatório para media_type audio"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Verifica se o valor de AudioBase64 é um JSON aninhado
		var nestedData struct {
			AudioBase64 string `json:"audio_base64"`
		}
		err := json.Unmarshal([]byte(req.AudioBase64), &nestedData)
		if err == nil && nestedData.AudioBase64 != "" {
			// Se for um JSON válido com o campo audio_base64, usa o valor interno
			inputData = nestedData.AudioBase64
		} else {
			// Caso contrário, usa o valor diretamente
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

	// Cria um diretório temporário para arquivos
	tempDir := "temp-" + fmt.Sprintf("%d", time.Now().UnixNano())
	err = os.Mkdir(tempDir, 0755)
	if err != nil {
		resp := TranscriptionResponse{Error: "Erro ao criar diretório temporário"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}
	log.Printf("Diretório temporário criado: %s", tempDir)
	defer os.RemoveAll(tempDir)

	// Salva o arquivo de entrada
	inputFile := tempDir + "/input"
	var outputFile string
	switch req.MediaType {
	case "audio":
		inputFile += ".ogg" // Tentaremos identificar o formato real depois
		outputFile = tempDir + "/output.wav"
	case "video":
		inputFile += ".mp4"
		outputFile = tempDir + "/audio.wav"
	case "image":
		inputFile += ".png"
	}

	// Converte os dados Base64 para arquivo
	data, err := base64.StdEncoding.DecodeString(inputData)
	if err != nil {
		resp := TranscriptionResponse{Error: "Dados Base64 inválidos"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}
	err = ioutil.WriteFile(inputFile, data, 0644)
	if err != nil {
		resp := TranscriptionResponse{Error: "Erro ao salvar arquivo de entrada"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Processa o arquivo com base no media_type
	var transcription string
	var language string
	switch req.MediaType {
	case "audio":
		// Identifica o formato do arquivo usando ffprobe
		start := time.Now()
		cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=format_name:stream=duration", "-of", "json", inputFile)
		ffprobeOutput, err := cmd.CombinedOutput()
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao identificar formato do arquivo: " + string(ffprobeOutput)}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		var ffprobeResult struct {
			Format struct {
				FormatName string `json:"format_name"`
				Duration   string `json:"duration"`
			} `json:"format"`
		}
		err = json.Unmarshal(ffprobeOutput, &ffprobeResult)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao parsear saída do ffprobe: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		log.Printf("Formato do arquivo: %s", ffprobeResult.Format.FormatName)

		// Verifica a duração do áudio
		duration, err := strconv.ParseFloat(ffprobeResult.Format.Duration, 64)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao obter duração do áudio: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if duration > 120 {
			log.Printf("Áudio excede 120 segundos (%.2f segundos), cortando para 120 segundos", duration)
			duration = 120
		}

		// Converte o arquivo para .wav
		ffmpegArgs := []string{"-i", inputFile, "-ar", "16000", "-ac", "1", "-vn", "-map", "0:a", "-af", "afftdn", outputFile}
		if duration == 120 {
			ffmpegArgs = append(ffmpegArgs, "-t", "120")
		}
		err = exec.Command("ffmpeg", ffmpegArgs...).Run()
		ffmpegDuration := time.Since(start)
		log.Printf("Tempo de conversão com ffmpeg: %v", ffmpegDuration)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao converter áudio: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Verifica se o arquivo output.wav foi gerado e não está vazio
		fileInfo, err := os.Stat(outputFile)
		if err != nil {
			resp := TranscriptionResponse{Error: "Arquivo de áudio convertido não encontrado: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if fileInfo.Size() == 0 {
			resp := TranscriptionResponse{Error: "Arquivo de áudio convertido está vazio"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Transcreve com Whisper
		start = time.Now()
		cmd = exec.Command("whisper", outputFile, "--model", "/usr/local/share/whisper-models/ggml-small.bin", "--language", "auto", "--output-json", "--threads", "2", "--best-of", "5", "--no-timestamps")
		cmd.Stderr = os.Stderr // Redireciona stderr para os logs do PM2
		output, err := cmd.Output() // Captura o stdout (transcrição bruta)
		whisperDuration := time.Since(start)
		log.Printf("Tempo de transcrição com Whisper: %v", whisperDuration)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao transcrever áudio: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Log da saída bruta do Whisper (transcrição bruta)
		log.Printf("Saída bruta do Whisper (stdout): %s", string(output))

		// Lê o arquivo JSON gerado pelo Whisper
		jsonFile := outputFile + ".json" // ex.: temp-1743093117118393698/output.wav.json
		jsonData, err := ioutil.ReadFile(jsonFile)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao ler arquivo JSON do Whisper: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Log do conteúdo do arquivo JSON
		log.Printf("Conteúdo do arquivo JSON do Whisper: %s", string(jsonData))

		// Parseia o JSON
		var whisperOutput struct {
			Result struct {
				Language string `json:"language"`
			} `json:"result"`
			Transcription []struct {
				Text string `json:"text"`
			} `json:"transcription"`
		}
		err = json.Unmarshal(jsonData, &whisperOutput)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao parsear arquivo JSON do Whisper: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Verifica se há transcrição disponível
		if len(whisperOutput.Transcription) == 0 {
			resp := TranscriptionResponse{Error: "Nenhuma transcrição encontrada no JSON do Whisper"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		transcription = whisperOutput.Transcription[0].Text
		language = whisperOutput.Result.Language
	case "video":
		// Identifica o formato do arquivo usando ffprobe
		start := time.Now()
		cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=format_name:stream=duration", "-of", "json", inputFile)
		ffprobeOutput, err := cmd.CombinedOutput()
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao identificar formato do arquivo: " + string(ffprobeOutput)}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		var ffprobeResult struct {
			Format struct {
				FormatName string `json:"format_name"`
				Duration   string `json:"duration"`
			} `json:"format"`
		}
		err = json.Unmarshal(ffprobeOutput, &ffprobeResult)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao parsear saída do ffprobe: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		log.Printf("Formato do arquivo: %s", ffprobeResult.Format.FormatName)

		// Verifica a duração do vídeo
		duration, err := strconv.ParseFloat(ffprobeResult.Format.Duration, 64)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao obter duração do vídeo: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if duration > 120 {
			log.Printf("Vídeo excede 120 segundos (%.2f segundos), cortando para 120 segundos", duration)
			duration = 120
		}

		// Extrai o áudio do vídeo
		ffmpegArgs := []string{"-i", inputFile, "-vn", "-ar", "16000", "-ac", "1", "-af", "afftdn", outputFile}
		if duration == 120 {
			ffmpegArgs = append(ffmpegArgs, "-t", "120")
		}
		err = exec.Command("ffmpeg", ffmpegArgs...).Run()
		ffmpegDuration := time.Since(start)
		log.Printf("Tempo de conversão com ffmpeg: %v", ffmpegDuration)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao extrair áudio do vídeo"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Verifica se o arquivo audio.wav foi gerado e não está vazio
		fileInfo, err := os.Stat(outputFile)
		if err != nil {
			resp := TranscriptionResponse{Error: "Arquivo de áudio convertido não encontrado: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if fileInfo.Size() == 0 {
			resp := TranscriptionResponse{Error: "Arquivo de áudio convertido está vazio"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Transcreve com Whisper
		start = time.Now()
		cmd := exec.Command("whisper", outputFile, "--model", "/usr/local/share/whisper-models/ggml-small.bin", "--language", "auto", "--output-json", "--threads", "2", "--best-of", "5", "--no-timestamps")
		cmd.Stderr = os.Stderr // Redireciona stderr para os logs do PM2
		output, err := cmd.Output() // Captura o stdout (transcrição bruta)
		whisperDuration := time.Since(start)
		log.Printf("Tempo de transcrição com Whisper: %v", whisperDuration)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao transcrever áudio do vídeo: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Log da saída bruta do Whisper (transcrição bruta)
		log.Printf("Saída bruta do Whisper (stdout): %s", string(output))

		// Lê o arquivo JSON gerado pelo Whisper
		jsonFile := outputFile + ".json" // ex.: temp-1743093117118393698/audio.wav.json
		jsonData, err := ioutil.ReadFile(jsonFile)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao ler arquivo JSON do Whisper: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Log do conteúdo do arquivo JSON
		log.Printf("Conteúdo do arquivo JSON do Whisper: %s", string(jsonData))

		// Parseia o JSON
		var whisperOutput struct {
			Result struct {
				Language string `json:"language"`
			} `json:"result"`
			Transcription []struct {
				Text string `json:"text"`
			} `json:"transcription"`
		}
		err = json.Unmarshal(jsonData, &whisperOutput)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao parsear arquivo JSON do Whisper: " + err.Error()}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Verifica se há transcrição disponível
		if len(whisperOutput.Transcription) == 0 {
			resp := TranscriptionResponse{Error: "Nenhuma transcrição encontrada no JSON do Whisper"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		transcription = whisperOutput.Transcription[0].Text
		language = whisperOutput.Result.Language
	case "image":
		// Realiza OCR com Tesseract
		cmd := exec.Command("tesseract", inputFile, "stdout", "-l", "por")
		output, err := cmd.CombinedOutput()
		if err != nil {
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
	audioFile := tempDir + "/response.wav"
	responseText := "Transcrição: " + transcription
	piperCmd := exec.Command("/www/wwwroot/dialogix/transcription-api/.venv/bin/piper", "--model", piperModel, "--output_file", audioFile)
	piperCmd.Stdin = strings.NewReader(responseText)
	output, err := piperCmd.CombinedOutput()
	if err != nil {
		resp := TranscriptionResponse{Error: fmt.Sprintf("Erro ao gerar áudio de resposta: %v - Output: %s - Command: %s", err, string(output), piperCmd.String())}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Lê o áudio gerado e converte para Base64
	audioData, err := ioutil.ReadFile(audioFile)
	if err != nil {
		resp := TranscriptionResponse{Error: "Erro ao ler áudio de resposta"}
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
	if err != nil {
		log.Fatal("Erro ao iniciar o servidor:", err)
	}
}