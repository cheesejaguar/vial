package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MatchingSystemPrompt is the system instruction sent to the LLM for every
// matching request. It constrains the model to produce only valid JSON,
// which is essential for reliable parsing — any prose response would fail
// [ParseMatchResponse].
const MatchingSystemPrompt = "You are a precise environment variable matching assistant. Respond only with valid JSON."

// MatchingPromptTemplate is the user prompt template used by [FormatMatchPrompt].
// The two %s verbs are filled with the requested key name and the newline-
// separated list of available vault keys.
//
// The rules encoded here mirror those of the deterministic tiers so the LLM
// can reason about the same conventions (framework prefixes, purpose synonyms,
// service name variants) and fill gaps that pattern matching missed.
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

// MatchResponse is the structured result parsed from a raw LLM completion.
// A Match value of "NO_MATCH" means the LLM could not identify a suitable
// vault key. Confidence is always in [0, 1]; values outside that range are
// rejected by [ParseMatchResponse] as invalid.
//
// Note: the matcher tier caps the effective confidence at 0.75 regardless of
// what the LLM reports, so a model that claims 1.0 confidence still cannot
// outrank a deterministic tier result.
type MatchResponse struct {
	Match      string  `json:"match"`      // vault key name, or "NO_MATCH"
	Confidence float64 `json:"confidence"` // [0.0, 1.0]; capped at 0.75 by the caller
	Reason     string  `json:"reason"`     // human-readable explanation, used for debug logging
}

// FormatMatchPrompt builds the user-turn prompt for a matching inference
// call by inserting requestedKey and the joined list of vaultKeys into
// [MatchingPromptTemplate].
func FormatMatchPrompt(requestedKey string, vaultKeys []string) string {
	keyList := strings.Join(vaultKeys, "\n")
	return fmt.Sprintf(MatchingPromptTemplate, requestedKey, keyList)
}

// ParseMatchResponse extracts a [MatchResponse] from raw LLM output. It
// handles two common model behaviours:
//
//  1. Plain JSON — parsed directly.
//  2. Markdown code fences (``` or ```json) — the fences are stripped before
//     parsing. Many models add these despite the system prompt instructing
//     otherwise.
//
// Returns an error if the JSON is malformed or if Confidence is outside
// [0, 1]. The raw string is truncated to 200 characters in error messages to
// keep log output manageable.
func ParseMatchResponse(raw string) (*MatchResponse, error) {
	cleaned := strings.TrimSpace(raw)

	// Strip markdown code fences if present. Models often wrap JSON in
	// ```json ... ``` even when instructed to return only JSON, so we strip
	// them defensively rather than treating the response as malformed.
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

	// Guard against models that hallucinate out-of-range confidence values.
	// The matcher relies on Confidence being in [0, 1] for comparisons.
	if resp.Confidence < 0 || resp.Confidence > 1 {
		return nil, fmt.Errorf("invalid confidence %f from LLM", resp.Confidence)
	}

	return &resp, nil
}

// truncateStr returns s unchanged if len(s) <= n, otherwise returns the first
// n bytes of s followed by "...". Used to keep error messages readable when
// LLM responses are unexpectedly long.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
