package llm

import (
	"testing"
)

func TestParseMatchResponse(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantMatch  string
		wantConf   float64
		wantReason string
		wantErr    bool
	}{
		{
			name:      "valid match",
			raw:       `{"match": "OPENAI_API_KEY", "confidence": 0.9, "reason": "common variant"}`,
			wantMatch: "OPENAI_API_KEY",
			wantConf:  0.9,
		},
		{
			name:      "no match",
			raw:       `{"match": "NO_MATCH", "confidence": 0.0, "reason": "no suitable match"}`,
			wantMatch: "NO_MATCH",
			wantConf:  0.0,
		},
		{
			name: "markdown code fence",
			raw: "```json\n{\"match\": \"STRIPE_KEY\", \"confidence\": 0.8, \"reason\": \"stripe key\"}\n```",
			wantMatch: "STRIPE_KEY",
			wantConf:  0.8,
		},
		{
			name:    "invalid json",
			raw:     "not json at all",
			wantErr: true,
		},
		{
			name:    "invalid confidence",
			raw:     `{"match": "KEY", "confidence": 1.5, "reason": "test"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseMatchResponse(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Match != tt.wantMatch {
				t.Errorf("Match = %q, want %q", resp.Match, tt.wantMatch)
			}
			if resp.Confidence != tt.wantConf {
				t.Errorf("Confidence = %f, want %f", resp.Confidence, tt.wantConf)
			}
		})
	}
}

func TestFormatMatchPrompt(t *testing.T) {
	prompt := FormatMatchPrompt("MY_KEY", []string{"OPENAI_API_KEY", "STRIPE_KEY"})
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
	if !contains(prompt, "MY_KEY") {
		t.Error("prompt should contain requested key")
	}
	if !contains(prompt, "OPENAI_API_KEY") {
		t.Error("prompt should contain vault keys")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
