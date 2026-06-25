package i18n

import (
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	prompt := Get("system.agents_browser_prompt")
	if !strings.Contains(prompt, "BrowserAgent") {
		t.Errorf("esperava prompt do browser contendo 'BrowserAgent', obteve: %q", prompt)
	}

	coderPrompt := Get("system.agents_coder_prompt")
	if !strings.Contains(coderPrompt, "CoderAgent") {
		t.Errorf("esperava prompt do coder contendo 'CoderAgent', obteve: %q", coderPrompt)
	}
}
