package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestServerInitialize verifies that the "initialize" request returns the
// correct server name and a non-nil tools capability.
func TestServerInitialize(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"test","version":"1.0"}}}` + "\n"
	output := &bytes.Buffer{}

	server := &Server{
		tools:  NewToolRegistry(nil, false, nil),
		input:  strings.NewReader(input),
		output: output,
	}

	if err := server.Serve(); err != nil {
		t.Fatal(err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("parsing response: %v\nraw: %s", err, output.String())
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.ID != float64(1) {
		t.Errorf("expected id 1, got %v", resp.ID)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result InitializeResult
	json.Unmarshal(resultBytes, &result)

	if result.ServerInfo.Name != "vial" {
		t.Errorf("expected server name 'vial', got %q", result.ServerInfo.Name)
	}
	if result.Capabilities.Tools == nil {
		t.Error("expected tools capability")
	}
}

// TestServerToolsList confirms that the read-only tool set contains exactly
// four tools and that enabling writes adds vault_set and vault_remove (six
// total).
func TestServerToolsList(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"
	output := &bytes.Buffer{}

	server := &Server{
		tools:  NewToolRegistry(nil, false, nil),
		input:  strings.NewReader(input),
		output: output,
	}

	if err := server.Serve(); err != nil {
		t.Fatal(err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result ToolsListResult
	json.Unmarshal(resultBytes, &result)

	// Should have read-only tools (no writes)
	if len(result.Tools) != 4 {
		t.Errorf("expected 4 read-only tools, got %d", len(result.Tools))
	}

	// With writes enabled
	input2 := `{"jsonrpc":"2.0","id":3,"method":"tools/list"}` + "\n"
	output2 := &bytes.Buffer{}
	server2 := &Server{
		tools:  NewToolRegistry(nil, true, nil),
		input:  strings.NewReader(input2),
		output: output2,
	}
	server2.Serve()

	var resp2 JSONRPCResponse
	json.Unmarshal(output2.Bytes(), &resp2)
	resultBytes2, _ := json.Marshal(resp2.Result)
	var result2 ToolsListResult
	json.Unmarshal(resultBytes2, &result2)

	if len(result2.Tools) != 6 {
		t.Errorf("expected 6 tools with writes, got %d", len(result2.Tools))
	}
}

// TestServerPing verifies that the "ping" method returns a successful empty
// result, confirming liveness check support.
func TestServerPing(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":4,"method":"ping"}` + "\n"
	output := &bytes.Buffer{}

	server := &Server{
		tools:  NewToolRegistry(nil, false, nil),
		input:  strings.NewReader(input),
		output: output,
	}

	server.Serve()

	var resp JSONRPCResponse
	json.Unmarshal(output.Bytes(), &resp)

	if resp.Error != nil {
		t.Errorf("ping should not error: %v", resp.Error)
	}
}

// TestServerUnknownMethod verifies that an unrecognised method name returns a
// -32601 "method not found" error, as required by JSON-RPC 2.0.
func TestServerUnknownMethod(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":5,"method":"nonexistent"}` + "\n"
	output := &bytes.Buffer{}

	server := &Server{
		tools:  NewToolRegistry(nil, false, nil),
		input:  strings.NewReader(input),
		output: output,
	}

	server.Serve()

	var resp JSONRPCResponse
	json.Unmarshal(output.Bytes(), &resp)

	if resp.Error == nil {
		t.Error("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", resp.Error.Code)
	}
}
