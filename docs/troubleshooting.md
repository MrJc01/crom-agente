# Guia de Resolução de Problemas (Troubleshooting) — crom-agente

Este guia lista os erros mais comuns e como solucioná-los ao desenvolver ou executar o **crom-agente**.

---

## 🛑 1. Portas Travadas (Conflitos de Rede)

### Sintoma
```
falha ao ligar API Server na porta 9090: address already in use
```
### Solução
O daemon utiliza as portas TCP `9090` (HTTP/WS API) e `9091` (gRPC). Se outra aplicação estiver usando-as:
1. Identifique o processo que ocupa a porta:
   ```bash
   lsof -i :9090
   ```
2. Pare o processo ou configure portas alternativas nas opções de inicialização do Daemon:
   ```bash
   # Modifique no arquivo de configuração ~/.crom/global.json
   ```

---

## 🔌 2. Soquetes e Arquivos de PID Travados

### Sintoma
```
daemon ja esta em execucao com PID xxxxx
```
### Causa
Se o daemon foi encerrado abruptamente (por exemplo, queda de energia ou sinal SIGKILL), o arquivo `crom-agente.pid` ou o arquivo de socket Unix `crom-agente.sock` podem ficar obsoletos no diretório `~/.crom/`.

### Solução
O crom-agente tenta limpar automaticamente stale PIDs, mas caso não consiga:
```bash
rm -f ~/.crom/crom-agente.pid
rm -f ~/.crom/crom-agente.sock
```

---

## 🔒 3. Erro de Sandbox / Acesso Negado (Jail Mode)

### Sintoma
```
acesso negado: o arquivo está fora do sandbox do workspace
```
### Causa
A configuração `workspace_jail` está habilitada como `true` e você (ou o agente ReAct) tentou ler/gravar em um caminho que resolve para fora do diretório do seu workspace ativo (Path Traversal).

### Solução
* Certifique-se de que os caminhos passados para ferramentas são relativos ou absolutos apontando para dentro do workspace.
* Caso realmente precise acessar arquivos fora da pasta do projeto, altere no `config.json` do workspace:
  ```json
  "workspace_jail": false
  ```

---

## 🤖 4. Ollama e Problemas de GPU

### Sintoma
* O agente fica excessivamente lento nas respostas.
* Erros de timeout de LLM ou conexões recusadas na porta `11434`.

### Solução
1. Certifique-se de que o serviço do Ollama está rodando localmente:
   ```bash
   curl http://127.0.0.1:11434
   ```
2. Verifique se o Ollama está utilizando aceleração por hardware (GPU). No Linux:
   ```bash
   nvidia-smi
   ```
3. Se a placa de vídeo não estiver sendo reconhecida pelo Ollama, inicialize-o explicitamente permitindo compatibilidade:
   ```bash
   # Em sistemas com Docker
   docker run -d --gpus=all -v ollama:/root/.ollama -p 11434:11434 ollama/ollama
   ```
