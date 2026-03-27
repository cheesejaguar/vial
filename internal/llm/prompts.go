package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MatchingSystemPrompt is the system prompt for env var matching.
const MatchingSystemPrompt = "You are a precise environment variable matching assistant. Respond only with valid JSON."

// MatchingPromptTemplate is the user prompt template for matching.
const MatchingPromptTemplate = `Given a requested environment variable name and a list of available vault keys, determine which vault key (if any) is the best match.

Rules:
- Consider common naming conventions (e.g., OPENAI_KEY and OPENAI_API_KEY are likely the same)
- Consider service name variations (e.g., POSTGRES, PG, POSTGRESQL)
- Consider purpose synonyms (KEY, TOKEN, SECRET, PASSWORD, CREDENTIAL)
- Framework prefixes like NEXT_PUBLIC_, VITE_, REACT_APP_ should be ignored when matching
- If no match is confident, say "NO_MATCH"

Respond in JSON format only:
{"match": "VAULT_KEY_NAME", "confidence": 0.85, "reason": "brief explanation"}
or
{"match": "NO_MATCH", "confidence": 0.0, "reason": "brief explanation"}

Requested key: %s
Available vault keys:
%s`

// MatchResponse is the structured result from an LLM match attempt.
type MatchResponse struct {
	Match      string  `json:"match"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// FormatMatchPrompt creates the user prompt for a matching request.
func FormatMatchPrompt(requestedKey string, vaultKeys []string) string {
	keyList := strings.Join(vaultKeys, "\n")
	return fmt.Sprintf(MatchingPromptTemplate, requestedKey, keyList)
}

// ParseMatchResponse extracts the structured match result from LLM output.
func ParseMatchResponse(raw string) (*MatchResponse, error) {
	cleaned := strings.TrimSpace(raw)

	// Strip markdown code fences if present
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		cleaned = strings.Join(jsonLines, "\n")
	}

	cleaned = strings.TrimSpace(cleaned)

	var resp MatchResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w (raw: %s)", err, truncateStr(raw, 200))
	}

	if resp.Confidence < 0 || resp.Confidence > 1 {
		return nil, fmt.Errorf("invalid confidence %f from LLM", resp.Confidence)
	}

	return &resp, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
