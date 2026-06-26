import sys

def process():
    with open("internal/llm/providers/openai.go", "r") as f:
        content = f.read()
    
    # We will replace the client.Do(req) block with the retry loop
    old = """	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("openai: erro ao criar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: falha na requisição HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: status HTTP inválido (%d): %s", resp.StatusCode, string(bodyBytes))
	}"""
    
    new = """	client := &http.Client{}
	var resp *http.Response
	var req *http.Request
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		var err error
		req, err = http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("openai: erro ao criar request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
		req.Header.Set("Accept", "text/event-stream")

		resp, err = client.Do(req)
		if err != nil {
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*2) * time.Second)
				continue
			}
			return nil, fmt.Errorf("openai: falha na requisição HTTP após retries: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*5) * time.Second)
				continue
			}
			break
		}

		if resp.StatusCode >= 500 {
			resp.Body.Close()
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*2) * time.Second)
				continue
			}
			break
		}

		break
	}

	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: status HTTP inválido (%d) (stream): %s", resp.StatusCode, string(bodyBytes))
	}"""
    
    if "for attempt :=" not in content.split("StreamMessages")[1]:
        content = content.replace(old, new)
        with open("internal/llm/providers/openai.go", "w") as f:
            f.write(content)
            
process()
