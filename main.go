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
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Valida e seleciona os dados de entrada com base no media_type
	var inputData string
	switch req.MediaType {
	case "audio":
		if req.AudioBase64 == "" {
			resp := TranscriptionResponse{Error: "audio_base64 é obrigatório para media_type audio"}
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
			json.NewEncoder(w).Encode(resp)
			return
		}
		inputData = req.VideoBase64
	case "image":
		if req.ImageBase64 == "" {
			resp := TranscriptionResponse{Error: "image_base64 é obrigatório para media_type image"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		inputData = req.ImageBase64
	default:
		resp := TranscriptionResponse{Error: "media_type inválido (deve ser audio, video ou image)"}
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Cria um diretório temporário para arquivos
	tempDir := "temp-" + fmt.Sprintf("%d", time.Now().UnixNano())
	err = os.Mkdir(tempDir, 0755)
	if err != nil {
		resp := TranscriptionResponse{Error: "Erro ao criar diretório temporário"}
		json.NewEncoder(w).Encode(resp)
		return
	}
	log.Printf("Diretório temporário criado: %s", tempDir)
	// Removido temporariamente para depuração
	// defer os.RemoveAll(tempDir)

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
		json.NewEncoder(w).Encode(resp)
		return
	}
	err = ioutil.WriteFile(inputFile, data, 0644)
	if err != nil {
		resp := TranscriptionResponse{Error: "Erro ao salvar arquivo de entrada"}
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
		cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=format_name", "-of", "json", inputFile)
		ffprobeOutput, err := cmd.CombinedOutput()
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao identificar formato do arquivo: " + string(ffprobeOutput)}
			json.NewEncoder(w).Encode(resp)
			return
		}
		var ffprobeResult struct {
			Format struct {
				FormatName string `json:"format_name"`
			} `json:"format"`
		}
		err = json.Unmarshal(ffprobeOutput, &ffprobeResult)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao parsear saída do ffprobe: " + err.Error()}
			json.NewEncoder(w).Encode(resp)
			return
		}
		log.Printf("Formato do arquivo: %s", ffprobeResult.Format.FormatName)

		// Converte o arquivo para .wav (limita a 30 segundos para evitar longos tempos de processamento)
		err = exec.Command("ffmpeg", "-i", inputFile, "-ar", "16000", "-ac", "1", "-vn", "-map", "0:a", "-t", "30", outputFile).Run()
		ffmpegDuration := time.Since(start)
		log.Printf("Tempo de conversão com ffmpeg: %v", ffmpegDuration)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao converter áudio: " + err.Error()}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Verifica se o arquivo output.wav foi gerado e não está vazio
		fileInfo, err := os.Stat(outputFile)
		if err != nil {
			resp := TranscriptionResponse{Error: "Arquivo de áudio convertido não encontrado: " + err.Error()}
			json.NewEncoder(w).Encode(resp)
			return
		}
		if fileInfo.Size() == 0 {
			resp := TranscriptionResponse{Error: "Arquivo de áudio convertido está vazio"}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Transcreve com Whisper e detecta o idioma
		start = time.Now()
		cmd = exec.Command("whisper", outputFile, "--model", "/usr/local/share/whisper-models/ggml-tiny.bin", "--language", "auto", "--output-json", "--threads", "4", "--best-of", "3", "--no-timestamps")
		cmd.Stderr = os.Stderr // Redireciona stderr para os logs do PM2
		output, err := cmd.Output() // Captura apenas o stdout
		whisperDuration := time.Since(start)
		log.Printf("Tempo de transcrição com Whisper: %v", whisperDuration)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao transcrever áudio: " + err.Error()}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Log da saída do Whisper diretamente no PM2
		log.Printf("Saída bruta do Whisper: %s", string(output))

		// Log da saída do Whisper para depuração
		err = ioutil.WriteFile(tempDir+"/whisper_output.log", output, 0644)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao salvar log da saída do Whisper: " + err.Error()}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Parseia a saída JSON do Whisper
		var whisperOutput struct {
			Language string `json:"language"`
			Text     string `json:"text"`
		}
		err = json.Unmarshal(output, &whisperOutput)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao parsear saída do Whisper: " + err.Error()}
			json.NewEncoder(w).Encode(resp)
			return
		}
		transcription = whisperOutput.Text
		language = whisperOutput.Language
	case "video":
		// Extrai o áudio do vídeo
		start := time.Now()
		err = exec.Command("ffmpeg", "-i", inputFile, "-vn", "-ar", "16000", "-ac", "1", "-t", "30", outputFile).Run()
		ffmpegDuration := time.Since(start)
		log.Printf("Tempo de conversão com ffmpeg: %v", ffmpegDuration)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao extrair áudio do vídeo"}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Verifica se o arquivo audio.wav foi gerado e não está vazio
		fileInfo, err := os.Stat(outputFile)
		if err != nil {
			resp := TranscriptionResponse{Error: "Arquivo de áudio convertido não encontrado: " + err.Error()}
			json.NewEncoder(w).Encode(resp)
			return
		}
		if fileInfo.Size() == 0 {
			resp := TranscriptionResponse{Error: "Arquivo de áudio convertido está vazio"}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Transcreve com Whisper e detecta o idioma
		start = time.Now()
		cmd := exec.Command("whisper", outputFile, "--model", "/usr/local/share/whisper-models/ggml-tiny.bin", "--language", "auto", "--output-json", "--threads", "4", "--best-of", "3", "--no-timestamps")
		cmd.Stderr = os.Stderr // Redireciona stderr para os logs do PM2
		output, err := cmd.Output() // Captura apenas o stdout
		whisperDuration := time.Since(start)
		log.Printf("Tempo de transcrição com Whisper: %v", whisperDuration)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao transcrever áudio do vídeo: " + err.Error()}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Log da saída do Whisper diretamente no PM2
		log.Printf("Saída bruta do Whisper: %s", string(output))

		// Log da saída do Whisper para depuração
		err = ioutil.WriteFile(tempDir+"/whisper_output.log", output, 0644)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao salvar log da saída do Whisper: " + err.Error()}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Parseia a saída JSON do Whisper
		var whisperOutput struct {
			Language string `json:"language"`
			Text     string `json:"text"`
		}
		err = json.Unmarshal(output, &whisperOutput)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao parsear saída do Whisper: " + err.Error()}
			json.NewEncoder(w).Encode(resp)
			return
		}
		transcription = whisperOutput.Text
		language = whisperOutput.Language
	case "image":
		// Realiza OCR com Tesseract
		cmd := exec.Command("tesseract", inputFile, "stdout", "-l", "por")
		output, err := cmd.CombinedOutput()
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao realizar OCR: " + string(output)}
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
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Lê o áudio gerado e converte para Base64
	audioData, err := ioutil.ReadFile(audioFile)
	if err != nil {
		resp := TranscriptionResponse{Error: "Erro ao ler áudio de resposta"}
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
	json.NewEncoder(w).Encode(resp)

	// Limpa o diretório temporário (reative após a depuração)
	// os.RemoveAll(tempDir)
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