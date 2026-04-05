package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cheesejaguar/vial/internal/audit"
	"github.com/cheesejaguar/vial/internal/vault"
)

// ToolRegistry manages the set of MCP tools available on this server
// instance. The available tool set is determined at construction time by the
// allowWrites flag; it does not change while the server is running.
type ToolRegistry struct {
	vm          *vault.VaultManager // vault access; may be nil in tests
	allowWrites bool                // when false, vault_set and vault_remove are not registered
	auditLog    *audit.Log          // optional; if nil, access events are not recorded
}

// NewToolRegistry constructs a [ToolRegistry]. Pass allowWrites = true only
// when the user has explicitly opted in via --allow-writes, because mutation
// tools increase the blast radius of a compromised MCP client. Pass nil for
// auditLog to disable audit logging (useful in tests).
func NewToolRegistry(vm *vault.VaultManager, allowWrites bool, auditLog *audit.Log) *ToolRegistry {
	return &ToolRegistry{vm: vm, allowWrites: allowWrites, auditLog: auditLog}
}

// ListTools returns the complete list of [ToolDefinition] values that this
// server advertises. The base set of four read-only tools is always present;
// vault_set and vault_remove are appended only when allowWrites is true.
//
// The returned slice is freshly allocated on each call and can be mutated
// freely by the caller.
func (r *ToolRegistry) ListTools() []ToolDefinition {
	tools := []ToolDefinition{
		{
			Name:        "vault_list",
			Description: "List all secret key names in the vault (values are not returned)",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "vault_get",
			Description: "Get a secret value from the vault by key name",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key": map[string]interface{}{
						"type":        "string",
						"description": "The secret key name (e.g. OPENAI_API_KEY)",
					},
				},
				"required": []string{"key"},
			},
		},
		{
			Name:        "vault_search",
			Description: "Search for secrets by key name pattern (case-insensitive substring match)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Substring to search for in key names",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "vault_health",
			Description: "Get the health status of all secrets (age, rotation status)",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}

	if r.allowWrites {
		tools = append(tools,
			ToolDefinition{
				Name:        "vault_set",
				Description: "Store a secret in the vault",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"key": map[string]interface{}{
							"type":        "string",
							"description": "The secret key name",
						},
						"value": map[string]interface{}{
							"type":        "string",
							"description": "The secret value",
						},
					},
					"required": []string{"key", "value"},
				},
			},
			ToolDefinition{
				Name:        "vault_remove",
				Description: "Remove a secret from the vault",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"key": map[string]interface{}{
							"type":        "string",
							"description": "The secret key name to remove",
						},
					},
					"required": []string{"key"},
				},
			},
		)
	}

	return tools
}

// CallTool dispatches a tool call by name and returns the result. Every
// call first checks that the vault is unlocked; a locked vault returns an
// error result rather than a JSON-RPC protocol error so the AI client can
// surface a human-readable message.
//
// Write tools (vault_set, vault_remove) return an error result when
// allowWrites is false, even if they were somehow called — this is a
// defence-in-depth check beyond the tool not being advertised in ListTools.
func (r *ToolRegistry) CallTool(name string, args map[string]interface{}) *CallToolResult {
	if !r.vm.IsUnlocked() {
		return errorResult("vault is locked — unlock it first with 'vial uncork'")
	}

	switch name {
	case "vault_list":
		return r.handleList()
	case "vault_get":
		return r.handleGet(args)
	case "vault_search":
		return r.handleSearch(args)
	case "vault_health":
		return r.handleHealth()
	case "vault_set":
		if !r.allowWrites {
			return errorResult("write operations not allowed — start server with --allow-writes")
		}
		return r.handleSet(args)
	case "vault_remove":
		if !r.allowWrites {
			return errorResult("write operations not allowed — start server with --allow-writes")
		}
		return r.handleRemove(args)
	default:
		return errorResult(fmt.Sprintf("unknown tool: %s", name))
	}
}

// handleList returns all key names, alphabetically sorted. Values are never
// included in the response, so this call is safe to make in any context.
func (r *ToolRegistry) handleList() *CallToolResult {
	keys, err := r.vm.VaultKeyNames()
	if err != nil {
		return errorResult(fmt.Sprintf("listing keys: %v", err))
	}
	sort.Strings(keys)

	if len(keys) == 0 {
		return textResult("No secrets in vault.")
	}

	return textResult(fmt.Sprintf("Vault contains %d secret(s):\n%s", len(keys), strings.Join(keys, "\n")))
}

// handleGet retrieves a single secret value. The caller (i.e. the AI model)
// receives the plaintext value, so every successful retrieval is recorded in
// the audit log with the "via MCP" tag.
func (r *ToolRegistry) handleGet(args map[string]interface{}) *CallToolResult {
	key, ok := args["key"].(string)
	if !ok || key == "" {
		return errorResult("'key' parameter is required")
	}

	val, err := r.vm.GetSecret(key)
	if err != nil {
		return errorResult(fmt.Sprintf("secret %q not found", key))
	}
	// Copy out of guarded memory before destroying the buffer; the string is
	// sent over the JSON-RPC transport and not retained by this package.
	result := string(val.Bytes())
	val.Destroy()

	if r.auditLog != nil {
		r.auditLog.Record(audit.EventGet, []string{key}, "", "via MCP")
	}

	return textResult(result)
}

// handleSearch performs a case-insensitive substring match against all key
// names. Only matching key names are returned — values are never included.
func (r *ToolRegistry) handleSearch(args map[string]interface{}) *CallToolResult {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errorResult("'query' parameter is required")
	}

	keys, err := r.vm.VaultKeyNames()
	if err != nil {
		return errorResult(fmt.Sprintf("listing keys: %v", err))
	}

	// Normalise to uppercase once rather than on every iteration.
	queryUpper := strings.ToUpper(query)
	var matches []string
	for _, key := range keys {
		if strings.Contains(strings.ToUpper(key), queryUpper) {
			matches = append(matches, key)
		}
	}

	if len(matches) == 0 {
		return textResult(fmt.Sprintf("No secrets matching %q", query))
	}

	return textResult(fmt.Sprintf("Found %d match(es):\n%s", len(matches), strings.Join(matches, "\n")))
}

// handleHealth computes the age and rotation status of every secret and
// returns a JSON array. Status values:
//   - "ok"      — within rotation window or no window set and age <= 90 days
//   - "aging"   — 90–180 days without a rotation window
//   - "stale"   — over 180 days without a rotation window
//   - "overdue" — past the explicitly configured rotation deadline
func (r *ToolRegistry) handleHealth() *CallToolResult {
	secrets := r.vm.ListSecrets()
	if len(secrets) == 0 {
		return textResult("No secrets in vault.")
	}

	type healthInfo struct {
		Key          string `json:"key"`
		AgeDays      int    `json:"age_days"`
		RotationDays int    `json:"rotation_days,omitempty"` // 0 means no rotation policy
		Status       string `json:"status"`
	}

	now := time.Now()
	var health []healthInfo
	for _, sec := range secrets {
		ageDays := int(now.Sub(sec.Metadata.Rotated).Hours() / 24)
		status := "ok"
		if sec.Metadata.RotationDays > 0 && ageDays > sec.Metadata.RotationDays {
			status = "overdue"
		} else if ageDays > 180 {
			status = "stale"
		} else if ageDays > 90 {
			status = "aging"
		}
		health = append(health, healthInfo{
			Key:          sec.Key,
			AgeDays:      ageDays,
			RotationDays: sec.Metadata.RotationDays,
			Status:       status,
		})
	}

	data, _ := json.MarshalIndent(health, "", "  ")
	return textResult(string(data))
}

// handleSet stores a new or updated secret. The value is placed in a
// memguard-protected buffer before being passed to the vault so it benefits
// from mlock'd memory during the encryption step.
func (r *ToolRegistry) handleSet(args map[string]interface{}) *CallToolResult {
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	if key == "" || value == "" {
		return errorResult("'key' and 'value' parameters are required")
	}

	// Wrap the value in a locked buffer so that the vault layer receives key
	// material in the expected form and can clear it from memory after use.
	val := newLockedBuffer([]byte(value))
	defer val.Destroy()

	if err := r.vm.SetSecret(key, val); err != nil {
		return errorResult(fmt.Sprintf("storing %s: %v", key, err))
	}

	if r.auditLog != nil {
		r.auditLog.Record(audit.EventSet, []string{key}, "", "via MCP")
	}

	return textResult(fmt.Sprintf("✓ Stored %s", key))
}

// handleRemove permanently deletes a secret from the vault.
func (r *ToolRegistry) handleRemove(args map[string]interface{}) *CallToolResult {
	key, _ := args["key"].(string)
	if key == "" {
		return errorResult("'key' parameter is required")
	}

	if err := r.vm.RemoveSecret(key); err != nil {
		return errorResult(fmt.Sprintf("removing %s: %v", key, err))
	}

	if r.auditLog != nil {
		r.auditLog.Record(audit.EventRemove, []string{key}, "", "via MCP")
	}

	return textResult(fmt.Sprintf("✓ Removed %s", key))
}

// textResult constructs a successful [CallToolResult] containing a single
// plain-text content block.
func textResult(text string) *CallToolResult {
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
	}
}

// errorResult constructs a [CallToolResult] that signals a tool-level failure.
// The IsError flag tells MCP clients to treat this as an error even though
// the JSON-RPC call itself succeeded. This distinction lets AI clients
// surface the error message to the user without triggering retry logic meant
// for protocol errors.
func errorResult(msg string) *CallToolResult {
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: msg}},
		IsError: true,
	}
}
