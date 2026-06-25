package llm

import "testing"

func TestGetCapabilities(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		wantToolUse bool
		wantVision  bool
	}{
		{
			name:        "gpt-4o",
			model:       "gpt-4o",
			wantToolUse: true,
			wantVision:  true,
		},
		{
			name:        "llama-3.2-3b",
			model:       "llama-3.2-3b",
			wantToolUse: false,
			wantVision:  false,
		},
		{
			name:        "OpenRouter llama-3.2-3b-instruct",
			model:       "meta-llama/llama-3.2-3b-instruct",
			wantToolUse: false,
			wantVision:  false,
		},
		{
			name:        "OpenRouter llama-3-8b-instruct",
			model:       "meta-llama/llama-3-8b-instruct",
			wantToolUse: true,
			wantVision:  false,
		},
		{
			name:        "Unknown model fallback",
			model:       "super-cool-model-9000",
			wantToolUse: true, // fallback assumes true
			wantVision:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := GetCapabilities(tt.model)
			if caps.ToolUse != tt.wantToolUse {
				t.Errorf("GetCapabilities(%s) ToolUse = %v, want %v", tt.model, caps.ToolUse, tt.wantToolUse)
			}
			if caps.Vision != tt.wantVision {
				t.Errorf("GetCapabilities(%s) Vision = %v, want %v", tt.model, caps.Vision, tt.wantVision)
			}
		})
	}
}
