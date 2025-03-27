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

## Uso

#### Endpoint Principal: ``/transcription``
* Método: POST
* Content-Type: application/json
* Corpo da Requisição:

    ```json
    {
      "audio_base64": "<BASE64_DO_AUDIO>",
      "video_base64": "<BASE64_DO_VIDEO>",
      "image_base64": "<BASE64_DA_IMAGEM>",
      "media_type": "audio" // ou "video" ou "image"
    }
    ```
    
    * Use apenas um dos campos ``audio_base64``, ``video_base64`` ou ``image_base64``, dependendo do ``media_type``.
    
* Resposta:
    ```json
    {
      "transcription": "Texto transcrito",
      "audio_response_base64": "<BASE64_DO_AUDIO_DE_RESPOSTA>",
      "language": "pt",
      "error": null
    }
    ```
* Exemplo de Requisição:

    ```bash
    curl -X POST https://api.dialogix.com.br/transcription \
      -H "Content-Type: application/json" \
      -d '{"audio_base64": "<BASE64_DO_.OGG>", "media_type": "audio"}'
    ```
    

## Integração com n8n e Webhooks

A API foi projetada para receber webhooks de workflows no n8n, substituindo integrações robóticas como o Typebot por um agente de IA mais robusto.

#### Configuração no n8n

1. Crie um Workflow no n8n:
    
    * Adicione um nó Webhook para receber eventos (ex.: mensagens de áudio ou texto de usuários).
    * Configure o nó Webhook para enviar requisições POST para ``https://api.dialogix.com.br/transcription``.

2. Estrutura do Webhook: 

    * O nó Webhook deve enviar o áudio em Base64 no formato esperado pela API (veja o exemplo de requisição acima). 
    * Use o nó HTTP Request no n8n para enviar a requisição POST para a API.
    
3. Processamento da Resposta:

    * A API retorna a transcrição e um áudio de resposta em Base64.
    * Use a transcrição para alimentar o agente de IA no n8n e o áudio de resposta para enviar de volta ao usuário (ex.: via WhatsApp).

#### Exemplo de Workflow no n8n

1. **Nó Webhook:** Recebe o áudio do usuário (ex.: via WhatsApp).
2. **Nó HTTP Request:** 
    * Método: POST
    * URL: ``http://localhost:3200/transcription``
    * Corpo: ``{"audio_base64": "{{$node['Webhook'].json['audio_base64']}}", "media_type": "audio"}``
3. Nó de Agente de IA: Usa a transcrição retornada para gerar uma resposta inteligente.
4. Nó de Envio: Envia a resposta do agente de IA e o audio_response_base64 de volta ao usuário.    
    
## Desempenho em Produção

#### Escalabilidade para Alta Demanda

Para lidar com picos de demanda (ex.: várias requisições simultâneas):

1. **Aumente o Número de Instâncias:**
    * Configure múltiplas instâncias da API com PM2:
    ```bash
    pm2 start transcription-api --name transcription-api --instances 4
    ```
    * Use um balanceador de carga (ex.: Nginx) para distribuir requisições entre as instâncias.

2. **Otimização de Tempo:**
    * Reduza o parâmetro --best-of do Whisper de 5 para 3 para diminuir o tempo de transcrição (teste o impacto na qualidade).
    * Desative temporariamente o Piper-TTS durante picos de demanda, retornando apenas a transcrição:
    ```go
    // Comente a geração de áudio no main.go
    audioResponseBase64 := ""
    ```
3. **Cache de Modelos:**
    * O Whisper carrega o modelo ``ggml-small.bin`` (487 MB) a cada requisição. Configure o Whisper para manter o modelo em memória entre requisições, se possível.

4. **Monitoramento:**
    * Use o PM2 para monitorar o desempenho:
    ```bash
    pm2 monit
    ```
    * Configure alertas para CPU e memória para evitar gargalos.

## Limitações e Otimizações Futuras

* **Limite de 120 Segundos:** Áudios acima de 120 segundos são cortados. Para suportar áudios mais longos, aumente o limite e ajuste o hardware.
* **Uso de CPU:** O Whisper roda na CPU (sem GPU). Para áudios longos e alta demanda, considere usar uma GPU para acelerar a transcrição.
* **Qualidade da Transcrição em Português:** Embora o modelo ggml-small.bin tenha melhorado a transcrição, ainda há erros (ex.: "Nejamão" e "presidências marinas"). Teste o modelo ggml-medium.bin para maior precisão, mas avalie o impacto no tempo de execução.
* **Detecção de Idioma:** A detecção automática (--language auto) funciona bem, mas o idioma retornado pode estar incorreto (ex.: áudio em inglês detectado como pt). Isso não afeta a transcrição, mas pode ser ajustado no futuro.

## Contribuição

1. Faça um fork do repositório.
2. Crie uma branch para sua feature: ``git checkout -b feature/nova-funcionalidade.``
3. Commit suas alterações: ``git commit -m "Adiciona nova funcionalidade".``
4. Envie para o repositório remoto: ``git push origin feature/nova-funcionalidade.``
5. Abra um Pull Request.

## Licença

MIT License. Veja o arquivo ``LICENSE`` para mais detalhes.