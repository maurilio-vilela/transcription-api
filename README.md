# Transcription API

Uma API em Go para transcrição de áudio, vídeo e OCR de imagens, utilizando Whisper, FFmpeg, Tesseract e Piper-TTS. A API é capaz de processar mídias recebidas do WhatsApp, detectar o idioma do áudio e gerar respostas em áudio no idioma correspondente.

## Funcionalidades
- **Transcrição de Áudio**: Transcreve áudios usando o Whisper, com detecção automática de idioma (ex.: português, inglês).
- **Transcrição de Vídeo**: Extrai áudio de vídeos com FFmpeg e transcreve com Whisper, com detecção de idioma.
- **OCR de Imagens**: Extrai texto de imagens usando Tesseract.
- **Geração de Áudio**: Gera áudio a partir do texto transcrito usando Piper-TTS (via Python), com modelos de voz específicos para o idioma detectado.

## Tecnologias Utilizadas
- **Go**: Linguagem principal da API, escolhida por seu baixo consumo de RAM e CPU.
- **Whisper (whisper.cpp)**: Para transcrição de áudio e detecção de idioma.
- **FFmpeg**: Para extrair áudio de vídeos.
- **Tesseract**: Para OCR em imagens.
- **Piper-TTS (Python)**: Para geração de áudio (Text-to-Speech), com modelos para português e inglês.

## Pré-requisitos
Antes de usar a API, você precisa instalar as seguintes ferramentas no servidor:

### Dependências do Sistema
1. **Go** (versão 1.22 ou superior):
    ```bash
    wget https://golang.org/dl/go1.22.1.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.22.1.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    source ~/.bashrc
    go version
    ```

2. Python 3 e pip:
    ```bash
    sudo apt update
    sudo apt install -y python3 python3-pip
    ```

3. FFmpeg (para extrair áudio de vídeos):
    ```bash
    sudo apt install ffmpeg
    ffmpeg -version
    ```

4. Tesseract (para OCR em imagens):
    ```bash
    sudo apt install tesseract-ocr
    sudo apt install libtesseract-dev
    sudo apt install tesseract-ocr-por
    tesseract --version
    ```

5. Piper-TTS (Python) (para geração de áudio):
    ```bash
    cd /www/wwwroot/dialogix/transcription-api
    python3 -m venv .venv
    source .venv/bin/activate
    pip install piper-tts
    # Teste o Piper-TTS
    echo 'Bem-vindo ao mundo da síntese de voz!' | .venv/bin/piper --model pt_BR-faber-medium --output_file bemvindo.wav
    echo 'Welcome to the world of speech synthesis!' | .venv/bin/piper --model en_US-lessac-medium --output_file welcome.wav
    ```

6. Whisper (whisper.cpp) (para transcrição):
    ```bash
    sudo apt install -y build-essential g++ libsndfile1-dev
    git clone https://github.com/ggerganov/whisper.cpp.git
    cd whisper.cpp
    make
    ./models/download-ggml-model.sh base
    sudo mkdir -p /usr/local/share/whisper-models/
    sudo mv build/bin/whisper-cli /usr/local/bin/whisper
    sudo mv models/ggml-base.bin /usr/local/share/whisper-models/
    whisper --help
    ```

## Instalação

1. Clone o repositório:
    ```bash
    git clone https://github.com/maurilio-vilela/transcription-api.git
    cd transcription-api
    ```

2. Inicialize o projeto Go:
    ```bash
    go mod init transcription-api
    go mod tidy
    ```

3. Configure o ambiente virtual para o Piper-TTS:
    ```bash
    python3 -m venv .venv
    source .venv/bin/activate
    pip install piper-tts
    ```

4. Compile a API:
    ```bash
    go build -o transcription-api main.go
    ```
5. Execute a API:
    ```bash
    ./transcription-api
    ```
* A API estará rodando na porta 3200.

6. (Opcional) Gerencie com PM2:
    ```bash
    pm2 start ./transcription-api --name transcription-api
    pm2 save
    ```