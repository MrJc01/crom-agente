package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type MediaExtractor struct {
	providerName string
	apiKey       string
	apiURL       string
}

func NewMediaExtractor(providerName, apiKey, apiURL string) *MediaExtractor {
	return &MediaExtractor{
		providerName: providerName,
		apiKey:       apiKey,
		apiURL:       apiURL,
	}
}

// Extract analisa o arquivo de mídia (imagem, áudio ou vídeo) e extrai contexto escrito (Layer 2)
func (me *MediaExtractor) Extract(ctx context.Context, mediaType, b64Data string) (string, error) {
	// Se for imagem, tenta usar um VLM leve
	if mediaType == "image" {
		desc, err := me.describeImageVLM(ctx, b64Data)
		if err == nil && desc != "" {
			return fmt.Sprintf("[Layer 2 VLM Extração]: %s", desc), nil
		}
		// Se falhar ou não tiver VLM, usa metadados predefinidos
		return me.extractImageMetadata(b64Data), nil
	}

	// Se for áudio, tenta transcrever com Vosk local
	if mediaType == "audio" {
		text, err := me.transcribeAudioLocal(b64Data)
		if err == nil && text != "" {
			return fmt.Sprintf("[Layer 2 Vosk Transcrição]: %s", text), nil
		}
		return me.extractAudioMetadata(b64Data), nil
	}

	// Se for vídeo ou outro
	return me.extractVideoMetadata(b64Data), nil
}

// describeImageVLM chama uma API rápida de VLM (ex: gemini-1.5-flash ou gpt-4o-mini) para descrever a imagem
func (me *MediaExtractor) describeImageVLM(ctx context.Context, b64Data string) (string, error) {
	if me.apiKey == "" {
		return "", fmt.Errorf("apiKey não configurada")
	}

	// Seleciona o modelo de visão mais rápido e barato
	vlmModel := "gpt-4o-mini"
	if me.providerName == "openrouter" || strings.Contains(me.apiURL, "openrouter.ai") {
		vlmModel = "google/gemini-2.5-flash"
	} else if me.providerName == "gemini" || strings.Contains(me.apiURL, "googleapis.com") {
		vlmModel = "gemini-1.5-flash"
	}

	prompt := "Descreva os elementos visuais importantes desta imagem de forma concisa e objetiva. Identifique qualquer texto, botões, ícones, erros ou elementos de interface presentes. Retorne apenas a descrição textual do que é visível."

	// Estrutura de request compatível com OpenAI
	type chatMessage struct {
		Role    string      `json:"role"`
		Content interface{} `json:"content"`
	}

	type openAIReq struct {
		Model    string        `json:"model"`
		Messages []chatMessage `json:"messages"`
	}

	var url string
	var reqBody []byte
	var err error

	if me.providerName == "gemini" || (strings.Contains(me.apiURL, "googleapis.com") && !strings.Contains(me.apiURL, "chat/completions")) {
		// API nativa do Gemini
		url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", vlmModel, me.apiKey)
		payload := map[string]interface{}{
			"contents": []interface{}{
				map[string]interface{}{
					"parts": []interface{}{
						map[string]interface{}{"text": prompt},
						map[string]interface{}{
							"inlineData": map[string]interface{}{
								"mimeType": "image/png",
								"data":     b64Data,
							},
						},
					},
				},
			},
		}
		reqBody, err = json.Marshal(payload)
	} else {
		// API OpenAI/OpenRouter/CromIA compatível
		url = me.apiURL
		if url == "" {
			url = "https://api.openai.com/v1/chat/completions"
		}
		payload := openAIReq{
			Model: vlmModel,
			Messages: []chatMessage{
				{
					Role: "user",
					Content: []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": prompt,
						},
						map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": "data:image/png;base64," + b64Data,
							},
						},
					},
				},
			},
		}
		reqBody, err = json.Marshal(payload)
	}

	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if !strings.Contains(url, "googleapis.com") {
		req.Header.Set("Authorization", "Bearer "+me.apiKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status VLM inválido (%d): %s", resp.StatusCode, string(respBytes))
	}

	if me.providerName == "gemini" || (strings.Contains(url, "googleapis.com") && !strings.Contains(url, "chat/completions")) {
		var geminiResp struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
			return "", err
		}
		if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
			return geminiResp.Candidates[0].Content.Parts[0].Text, nil
		}
	} else {
		var openAIResp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(respBytes, &openAIResp); err != nil {
			return "", err
		}
		if len(openAIResp.Choices) > 0 {
			return openAIResp.Choices[0].Message.Content, nil
		}
	}

	return "", fmt.Errorf("resposta VLM vazia")
}

// transcribeAudioLocal chama o script python de transcrição Vosk
func (me *MediaExtractor) transcribeAudioLocal(b64Data string) (string, error) {
	audioBytes, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return "", err
	}

	tempWav := filepath.Join(os.TempDir(), fmt.Sprintf("crom_media_extract_%d.wav", time.Now().UnixNano()))
	if err := os.WriteFile(tempWav, audioBytes, 0644); err != nil {
		return "", err
	}
	defer os.Remove(tempWav)

	possiblePaths := []string{
		"/home/j/Documentos/GitHub/crom-agente/scripts/transcribe.py",
		"/home/j/Área de trabalho/GitHub/crom-agente5/crom-agente/scripts/transcribe.py",
		"./scripts/transcribe.py",
	}

	var scriptPath string
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			scriptPath = p
			break
		}
	}

	if scriptPath == "" {
		return "", fmt.Errorf("script transcribe.py não encontrado")
	}

	cmd := exec.Command("python3", scriptPath, tempWav)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("vosk run error: %s", stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// extractImageMetadata extrai informações estruturadas da imagem base64
func (me *MediaExtractor) extractImageMetadata(b64Data string) string {
	rawSize := len(b64Data)
	approxBytes := (rawSize * 3) / 4

	width, height := 0, 0
	data, err := base64.StdEncoding.DecodeString(b64Data)
	if err == nil && len(data) > 24 && string(data[1:4]) == "PNG" {
		width = int(data[16])<<24 | int(data[17])<<16 | int(data[18])<<8 | int(data[19])
		height = int(data[20])<<24 | int(data[21])<<16 | int(data[22])<<8 | int(data[23])
	}

	if width > 0 && height > 0 {
		return fmt.Sprintf("[Layer 2 Estrutura Predefinida - Imagem]: Tipo: PNG, Resolução: %dx%d pixels, Tamanho Estimado: %.2f KB", width, height, float64(approxBytes)/1024.0)
	}
	return fmt.Sprintf("[Layer 2 Estrutura Predefinida - Imagem]: Tipo: Imagem, Tamanho Estimado: %.2f KB", float64(approxBytes)/1024.0)
}

// extractAudioMetadata extrai informações estruturadas do áudio base64
func (me *MediaExtractor) extractAudioMetadata(b64Data string) string {
	rawSize := len(b64Data)
	approxBytes := (rawSize * 3) / 4
	return fmt.Sprintf("[Layer 2 Estrutura Predefinida - Áudio]: Tipo: Áudio/WAV, Tamanho Estimado: %.2f KB (Transcrição offline indisponível)", float64(approxBytes)/1024.0)
}

// extractVideoMetadata extrai informações estruturadas do vídeo base64
func (me *MediaExtractor) extractVideoMetadata(b64Data string) string {
	rawSize := len(b64Data)
	approxBytes := (rawSize * 3) / 4
	return fmt.Sprintf("[Layer 2 Estrutura Predefinida - Vídeo]: Tipo: Vídeo/WebM, Tamanho Estimado: %.2f KB", float64(approxBytes)/1024.0)
}

// ExtractAndInjectMediaContext analisa mensagens do usuário, extrai texto via Layer 2 (MediaExtractor)
// e injeta o texto na mensagem do usuário para que a IA sempre receba o contexto mesmo se o payload nativo falhar.
func ExtractAndInjectMediaContext(ctx context.Context, messages []Message, providerName, apiKey, apiURL string) []Message {
	extractor := NewMediaExtractor(providerName, apiKey, apiURL)
	injectedMessages := make([]Message, len(messages))
	copy(injectedMessages, messages)

	for i, m := range injectedMessages {
		if m.Role != "user" || m.Content == "" {
			continue
		}

		// Evita re-processar mensagens que já possuem o contexto textual do Layer 2
		if strings.Contains(m.Content, "[Layer 2 ") {
			continue
		}

		var extractions []string
		lines := strings.Split(m.Content, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "image:base64:") {
				b64 := strings.TrimPrefix(trimmed, "image:base64:")
				desc, err := extractor.Extract(ctx, "image", b64)
				if err == nil && desc != "" {
					extractions = append(extractions, desc)
				}
			} else if strings.HasPrefix(trimmed, "audio:base64:") {
				b64 := strings.TrimPrefix(trimmed, "audio:base64:")
				parts := strings.SplitN(b64, ":", 2)
				actualB64 := b64
				if len(parts) == 2 {
					actualB64 = parts[1]
				}
				desc, err := extractor.Extract(ctx, "audio", actualB64)
				if err == nil && desc != "" {
					extractions = append(extractions, desc)
				}
			}
		}

		if len(extractions) > 0 {
			m.Content = m.Content + "\n\n" + strings.Join(extractions, "\n")
			injectedMessages[i] = m
		}
	}

	return injectedMessages
}

// StripMultimodalPayloads remove elementos nativos de visão/áudio do content, mantendo apenas texto.
// Suporta tanto string quanto []interface{} (estrutura multipart compatível com OpenAI/Gemini).
func StripMultimodalPayloads(content interface{}) interface{} {
	if content == nil {
		return nil
	}

	switch val := content.(type) {
	case []interface{}:
		var newParts []interface{}
		for _, part := range val {
			partMap, ok := part.(map[string]interface{})
			if ok {
				t, _ := partMap["type"].(string)
				if t == "text" {
					newParts = append(newParts, part)
				}
			} else {
				newParts = append(newParts, part)
			}
		}
		if len(newParts) == 0 {
			newParts = append(newParts, map[string]interface{}{
				"type": "text",
				"text": "[Mídia nativa omitida para compatibilidade]",
			})
		}
		return newParts

	case string:
		var newLines []string
		lines := strings.Split(val, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "image:base64:") && !strings.HasPrefix(trimmed, "audio:base64:") {
				newLines = append(newLines, line)
			} else {
				newLines = append(newLines, "[Mídia nativa omitida para compatibilidade]")
			}
		}
		return strings.Join(newLines, "\n")
	}

	return content
}
