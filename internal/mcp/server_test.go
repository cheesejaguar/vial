package mcp

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awnumar/memguard"

	"github.com/cheesejaguar/vial/internal/audit"
	"github.com/cheesejaguar/vial/internal/vault"
)

// serve runs a Server over a single newline-terminated request and returns the
// decoded JSON-RPC response.
func serve(t *testing.T, tools *ToolRegistry, request string) JSONRPCResponse {
	t.Helper()
	output := &bytes.Buffer{}
	server := &Server{
		tools:  tools,
		input:  strings.NewReader(request + "\n"),
		output: output,
	}
	if err := server.Serve(); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("parsing response: %v\nraw: %s", err, output.String())
	}
	return resp
}

// decodeResult re-marshals resp.Result into out.
func decodeResult(t *testing.T, resp JSONRPCResponse, out any) {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
}

// newTestVault returns an unlocked vault (fast test KDF) holding the given
// secrets.
func newTestVault(t *testing.T, secrets map[string]string) *vault.VaultManager {
	t.Helper()
	vm := vault.NewVaultManager(filepath.Join(t.TempDir(), "vault.json"))
	vm.SetKDFParams(vault.TestKDFParams())

	pw := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer pw.Destroy()
	if err := vm.Init(pw); err != nil {
		t.Fatalf("Init vault: %v", err)
	}
	for k, v := range secrets {
		buf := memguard.NewBufferFromBytes([]byte(v))
		if err := vm.SetSecret(k, buf); err != nil {
			t.Fatalf("SetSecret %s: %v", k, err)
		}
		buf.Destroy()
	}
	t.Cleanup(vm.Lock)
	return vm
}

// callTool sends a tools/call request and returns the decoded tool result.
func callTool(t *testing.T, tools *ToolRegistry, name string, args map[string]any) CallToolResult {
	t.Helper()
	req, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      10,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		t.Fatal(err)
	}
	var result CallToolResult
	decodeResult(t, serve(t, tools, string(req)), &result)
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("expected a single text content block, got %+v", result.Content)
	}
	return result
}

// TestServerInitialize verifies that the "initialize" request returns the
// correct server name, protocol version, and a non-nil tools capability.
func TestServerInitialize(t *testing.T) {
	resp := serve(t, NewToolRegistry(nil, false, nil),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"test","version":"1.0"}}}`)

	if resp.ID != float64(1) {
		t.Errorf("expected id 1, got %v", resp.ID)
	}

	var result InitializeResult
	decodeResult(t, resp, &result)

	if result.ServerInfo.Name != "vial" {
		t.Errorf("expected server name 'vial', got %q", result.ServerInfo.Name)
	}
	if result.ProtocolVersion != protocolVersion {
		t.Errorf("protocolVersion = %q, want %q", result.ProtocolVersion, protocolVersion)
	}
	if result.Capabilities.Tools == nil {
		t.Error("expected tools capability")
	}
}

// TestServerToolsList confirms that the read-only tool set contains exactly
// the four read tools and that enabling writes adds vault_set and
// vault_remove.
func TestServerToolsList(t *testing.T) {
	names := func(allowWrites bool) []string {
		var result ToolsListResult
		decodeResult(t, serve(t, NewToolRegistry(nil, allowWrites, nil), `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`), &result)
		var out []string
		for _, tool := range result.Tools {
			out = append(out, tool.Name)
		}
		return out
	}

	readOnly := strings.Join(names(false), ",")
	if readOnly != "vault_list,vault_get,vault_search,vault_health" {
		t.Errorf("read-only tools = %s", readOnly)
	}

	withWrites := strings.Join(names(true), ",")
	if withWrites != "vault_list,vault_get,vault_search,vault_health,vault_set,vault_remove" {
		t.Errorf("tools with writes = %s", withWrites)
	}
}

// TestServerPing verifies that the "ping" method returns a successful empty
// object result, confirming liveness check support.
func TestServerPing(t *testing.T) {
	resp := serve(t, NewToolRegistry(nil, false, nil), `{"jsonrpc":"2.0","id":4,"method":"ping"}`)

	if resp.ID != float64(4) {
		t.Errorf("expected id 4, got %v", resp.ID)
	}
	var result map[string]any
	decodeResult(t, resp, &result)
	if result == nil || len(result) != 0 {
		t.Errorf("ping result = %v, want empty object", result)
	}
}

// TestServerUnknownMethod verifies that an unrecognised method name returns a
// -32601 "method not found" error, as required by JSON-RPC 2.0.
func TestServerUnknownMethod(t *testing.T) {
	resp := serve(t, NewToolRegistry(nil, false, nil), `{"jsonrpc":"2.0","id":5,"method":"nonexistent"}`)

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", resp.Error.Code)
	}
}

// TestServerParseError verifies that malformed JSON yields a -32700 parse
// error instead of terminating the server.
func TestServerParseError(t *testing.T) {
	resp := serve(t, NewToolRegistry(nil, false, nil), `{not json`)

	if resp.Error == nil || resp.Error.Code != -32700 {
		t.Fatalf("expected -32700 parse error, got %+v", resp.Error)
	}
}

// TestToolsCallListAndSearch verifies vault_list returns sorted key names and
// vault_search does a case-insensitive substring match, neither exposing
// values.
func TestToolsCallListAndSearch(t *testing.T) {
	vm := newTestVault(t, map[string]string{
		"STRIPE_KEY":     "sk_live_1",
		"OPENAI_API_KEY": "sk-openai",
	})
	tools := NewToolRegistry(vm, false, nil)

	list := callTool(t, tools, "vault_list", nil)
	if list.IsError {
		t.Fatalf("vault_list error: %s", list.Content[0].Text)
	}
	if want := "Vault contains 2 secret(s):\nOPENAI_API_KEY\nSTRIPE_KEY"; list.Content[0].Text != want {
		t.Errorf("vault_list = %q, want %q", list.Content[0].Text, want)
	}

	search := callTool(t, tools, "vault_search", map[string]any{"query": "openai"})
	if want := "Found 1 match(es):\nOPENAI_API_KEY"; search.Content[0].Text != want {
		t.Errorf("vault_search = %q, want %q", search.Content[0].Text, want)
	}
	if strings.Contains(list.Content[0].Text+search.Content[0].Text, "sk-") {
		t.Error("list/search must not expose secret values")
	}
}

// TestToolsCallGetRecordsAudit verifies vault_get returns the plaintext value
// and records an audit event tagged "via MCP".
func TestToolsCallGetRecordsAudit(t *testing.T) {
	vm := newTestVault(t, map[string]string{"API_KEY": "secret-123"})
	auditLog := audit.NewLog(filepath.Join(t.TempDir(), "audit.jsonl"))
	tools := NewToolRegistry(vm, false, auditLog)

	got := callTool(t, tools, "vault_get", map[string]any{"key": "API_KEY"})
	if got.IsError || got.Content[0].Text != "secret-123" {
		t.Fatalf("vault_get = %+v, want secret-123", got)
	}

	entries, err := auditLog.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Event != audit.EventGet || entries[0].Detail != "via MCP" {
		t.Errorf("audit entries = %+v, want one get event via MCP", entries)
	}

	missing := callTool(t, tools, "vault_get", map[string]any{"key": "NOPE"})
	if !missing.IsError {
		t.Error("vault_get for a missing key should be an error result")
	}
	noArg := callTool(t, tools, "vault_get", nil)
	if !noArg.IsError {
		t.Error("vault_get without 'key' should be an error result")
	}
}

// TestToolsCallWritesGated verifies vault_set/vault_remove are refused when
// writes are disabled and change the vault when they are enabled.
func TestToolsCallWritesGated(t *testing.T) {
	vm := newTestVault(t, map[string]string{"OLD_KEY": "old"})

	readOnly := NewToolRegistry(vm, false, nil)
	if res := callTool(t, readOnly, "vault_set", map[string]any{"key": "NEW_KEY", "value": "v"}); !res.IsError {
		t.Error("vault_set should be refused without --allow-writes")
	}
	if _, err := vm.GetSecret("NEW_KEY"); err != vault.ErrSecretNotFound {
		t.Errorf("NEW_KEY should not exist after refused write, got err=%v", err)
	}

	writable := NewToolRegistry(vm, true, nil)
	if res := callTool(t, writable, "vault_set", map[string]any{"key": "NEW_KEY", "value": "v"}); res.IsError {
		t.Fatalf("vault_set: %s", res.Content[0].Text)
	}
	val, err := vm.GetSecret("NEW_KEY")
	if err != nil {
		t.Fatalf("GetSecret NEW_KEY: %v", err)
	}
	if string(val.Bytes()) != "v" {
		t.Errorf("NEW_KEY = %q, want v", val.Bytes())
	}
	val.Destroy()

	if res := callTool(t, writable, "vault_remove", map[string]any{"key": "OLD_KEY"}); res.IsError {
		t.Fatalf("vault_remove: %s", res.Content[0].Text)
	}
	if _, err := vm.GetSecret("OLD_KEY"); err != vault.ErrSecretNotFound {
		t.Errorf("OLD_KEY should be removed, got err=%v", err)
	}
}

// TestToolsCallLockedVault verifies every tool call returns an error result
// when the vault is locked.
func TestToolsCallLockedVault(t *testing.T) {
	vm := newTestVault(t, map[string]string{"API_KEY": "secret"})
	vm.Lock()

	res := callTool(t, NewToolRegistry(vm, true, nil), "vault_get", map[string]any{"key": "API_KEY"})
	if !res.IsError || !strings.Contains(res.Content[0].Text, "locked") {
		t.Errorf("result = %+v, want a 'vault is locked' error", res)
	}
}
