package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cheesejaguar/vial/internal/vault"
)

// ToolRegistry manages the available MCP tools.
type ToolRegistry struct {
	vm          *vault.VaultManager
	allowWrites bool
}

// NewToolRegistry creates a new tool registry.
func NewToolRegistry(vm *vault.VaultManager, allowWrites bool) *ToolRegistry {
	return &ToolRegistry{vm: vm, allowWrites: allowWrites}
}

// ListTools returns all available tool definitions.
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

// CallTool executes a tool and returns the result.
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

func (r *ToolRegistry) handleGet(args map[string]interface{}) *CallToolResult {
	key, ok := args["key"].(string)
	if !ok || key == "" {
		return errorResult("'key' parameter is required")
	}

	val, err := r.vm.GetSecret(key)
	if err != nil {
		return errorResult(fmt.Sprintf("secret %q not found", key))
	}
	result := string(val.Bytes())
	val.Destroy()

	return textResult(result)
}

func (r *ToolRegistry) handleSearch(args map[string]interface{}) *CallToolResult {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errorResult("'query' parameter is required")
	}

	keys, err := r.vm.VaultKeyNames()
	if err != nil {
		return errorResult(fmt.Sprintf("listing keys: %v", err))
	}

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

func (r *ToolRegistry) handleHealth() *CallToolResult {
	secrets := r.vm.ListSecrets()
	if len(secrets) == 0 {
		return textResult("No secrets in vault.")
	}

	type healthInfo struct {
		Key          string `json:"key"`
		AgeDays      int    `json:"age_days"`
		RotationDays int    `json:"rotation_days,omitempty"`
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

func (r *ToolRegistry) handleSet(args map[string]interface{}) *CallToolResult {
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	if key == "" || value == "" {
		return errorResult("'key' and 'value' parameters are required")
	}

	// Use memguard for the value
	val := newLockedBuffer([]byte(value))
	defer val.Destroy()

	if err := r.vm.SetSecret(key, val); err != nil {
		return errorResult(fmt.Sprintf("storing %s: %v", key, err))
	}

	return textResult(fmt.Sprintf("✓ Stored %s", key))
}

func (r *ToolRegistry) handleRemove(args map[string]interface{}) *CallToolResult {
	key, _ := args["key"].(string)
	if key == "" {
		return errorResult("'key' parameter is required")
	}

	if err := r.vm.RemoveSecret(key); err != nil {
		return errorResult(fmt.Sprintf("removing %s: %v", key, err))
	}

	return textResult(fmt.Sprintf("✓ Removed %s", key))
}

func textResult(text string) *CallToolResult {
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
	}
}

func errorResult(msg string) *CallToolResult {
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: msg}},
		IsError: true,
	}
}
