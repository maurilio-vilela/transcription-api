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
	Transcription      string `json:"transcription"`            // Texto transcrito ou extraído (OCR)
	AudioResponseBase64 string `json:"audio_response_base64,omitempty"` // Áudio de resposta gerado em Base64
	Language           string `json:"language"`                 // Idioma detectado (ex.: "pt", "en")
	Error              string `json:"error,omitempty"`          // Mensagem de erro, se houver
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
		inputData = req.AudioBase64
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
	defer os.RemoveAll(tempDir)

	// Salva o arquivo de entrada
	inputFile := tempDir + "/input"
	var outputFile string
	switch req.MediaType {
	case "audio":
		inputFile += ".ogg"
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
		// Converte o áudio para WAV (formato suportado pelo Whisper)
		err = exec.Command("ffmpeg", "-i", inputFile, "-ar", "16000", "-ac", "1", outputFile).Run()
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao converter áudio"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Transcreve com Whisper e detecta o idioma
		jsonOutputFile := tempDir + "/transcription.json"
		cmd := exec.Command("whisper", outputFile, "--model", "/usr/local/share/whisper-models/ggml-base.bin", "--language", "auto", "--output-json", "--output-file", tempDir+"/transcription")
		output, err := cmd.CombinedOutput()
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao transcrever áudio: " + string(output)}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Lê o arquivo JSON gerado pelo Whisper
		jsonData, err := ioutil.ReadFile(jsonOutputFile)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao ler arquivo JSON do Whisper"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Parseia a saída JSON do Whisper
		var whisperOutput struct {
			Transcription []struct {
				Text string `json:"text"`
			} `json:"transcription"`
			Language string `json:"language"`
		}
		err = json.Unmarshal(jsonData, &whisperOutput)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao parsear saída do Whisper"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Combina o texto de todas as transcrições
		var transcriptionParts []string
		for _, t := range whisperOutput.Transcription {
			transcriptionParts = append(transcriptionParts, t.Text)
		}
		transcription = strings.Join(transcriptionParts, " ")
		language = whisperOutput.Language
	case "video":
		// Extrai o áudio do vídeo
		err = exec.Command("ffmpeg", "-i", inputFile, "-vn", "-ar", "16000", "-ac", "1", outputFile).Run()
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao extrair áudio do vídeo"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Transcreve com Whisper e detecta o idioma
		jsonOutputFile := tempDir + "/transcription.json"
		cmd := exec.Command("whisper", outputFile, "--model", "/usr/local/share/whisper-models/ggml-base.bin", "--language", "auto", "--output-json", "--output-file", tempDir+"/transcription")
		output, err := cmd.CombinedOutput()
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao transcrever áudio do vídeo: " + string(output)}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Lê o arquivo JSON gerado pelo Whisper
		jsonData, err := ioutil.ReadFile(jsonOutputFile)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao ler arquivo JSON do Whisper"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Parseia a saída JSON do Whisper
		var whisperOutput struct {
			Transcription []struct {
				Text string `json:"text"`
			} `json:"transcription"`
			Language string `json:"language"`
		}
		err = json.Unmarshal(jsonData, &whisperOutput)
		if err != nil {
			resp := TranscriptionResponse{Error: "Erro ao parsear saída do Whisper"}
			json