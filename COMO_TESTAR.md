# Guia de Teste e Execução — crom-agente

Este documento explica como compilar, iniciar e testar o backend Go (daemon) e o aplicativo frontend Tauri de forma integrada.

---

## 🚀 1. Executando o Backend (Daemon Go)

O backend do `crom-agente` gerencia o servidor HTTP/WS local, a execução isolada de comandos, e o controle de gravações de tela e áudio.

### A. Compilação do Daemon
Entre na pasta do backend e execute a compilação:
```bash
cd crom-agente
```

* **Modo Headless (Sem interface/bandeja - recomendado para testes locais e servidores)**:
  Bypassa as dependências da biblioteca nativa GTK/Systray:
  ```bash
  go build -tags headless -o ./bin/crom-agente ./cmd/crom-agente
  ```

* **Modo Completo (Com bandeja do sistema - requer pacotes nativos de UI)**:
  ```bash
  go build -o ./bin/crom-agente ./cmd/crom-agente
  ```

### B. Inicialização do Daemon
Para testar a integração com o frontend sem restrições de login/token, você pode desabilitar a autenticação de sessão passando a variável de ambiente `CROM_DISABLE_AUTH=true`:

```bash
# Iniciar em modo headless e desabilitando autenticação
CROM_DISABLE_AUTH=true ./bin/crom-agente daemon start --headless
```

O daemon rodará em segundo plano escutando na porta padrão `9090` (HTTP) e `9091` (gRPC).

---

## 💻 2. Executando o Frontend (Tauri & Web)

O aplicativo frontend interage com o daemon local. Ele pode ser executado como uma janela nativa via Tauri ou no navegador comum.

### A. Preparação do Ambiente
Entre no diretório do aplicativo frontend e instale as dependências caso ainda não o tenha feito:
```bash
cd crom-agente-app
npm install
```

### B. Modo de Desenvolvimento (Tauri App)
Para rodar o aplicativo nativo com hot-reload (recarregamento automático ao salvar código):
```bash
npm run tauri dev
```
Isso abrirá a interface nativa do Tauri. 

### C. Modo Web (Navegador)
Se preferir rodar apenas o servidor Vite no navegador web (Chrome/Firefox):
```bash
npm run dev
```
E acesse a URL indicada (geralmente `http://localhost:1420`).

---

## 🎙️ 3. Testando as Funcionalidades de Mídia

### A. Gravação de Áudio & Transcrição Offline (Vosk)
1. Para realizar a transcrição offline sem depender de chaves de API pagas (como OpenAI Whisper), instale a biblioteca Vosk no Python do seu sistema:
   ```bash
   pip install vosk
   ```
2. O script local [transcribe.py](file:///home/j/Área de trabalho/GitHub/crom-agente5/crom-agente/scripts/transcribe.py) baixará automaticamente o modelo leve em português de 31MB (`vosk-model-small-pt-0.3`) na primeira execução e o salvará em `~/.crom/`.
3. Abra as **Configurações** (ícone de engrenagem) -> **Mídia & Gravação**.
4. Verifique se a lista de microfones locais foi carregada e selecione o dispositivo desejado.
5. Inicie a gravação clicando no ícone do microfone, fale e clique em parar para testar a transcrição automática.

### B. Compartilhamento e Escolha de Tela/Janela
1. Clique no botão de **Gravar Tela** (ícone de Monitor).
2. Se você estiver rodando no **aplicativo Tauri** (com o daemon ativo):
   - Um modal interativo abrirá listando os seus monitores físicos disponíveis (detectados via `xrandr`) e todas as janelas que você tem abertas no seu sistema operacional (detectadas via `wmctrl`).
   - Escolha o que deseja gravar. O backend usará a ferramenta `gst-launch-1.0` nativa para gravar apenas o item escolhido.
3. Se estiver rodando no **Navegador**:
   - O app utilizará o picker nativo do próprio navegador (WebRTC `getDisplayMedia`) para que você escolha a aba ou tela.
