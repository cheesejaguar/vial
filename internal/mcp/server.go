package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/awnumar/memguard"
	"github.com/cheesejaguar/vial/internal/audit"
	"github.com/cheesejaguar/vial/internal/vault"
)

// Protocol and identity constants advertised to connecting clients during
// the "initialize" handshake. These are embedded in [InitializeResult] and
// must not be changed without verifying compatibility with targeted MCP
// client versions.
const (
	protocolVersion = "2024-11-05" // MCP spec version this server implements
	serverName      = "vial"
	serverVersion   = "0.1.0"
)

// Server implements an MCP server using the JSON-RPC 2.0 stdio transport.
// Each newline-delimited JSON message on input is dispatched to the
// appropriate handler and a response is written back as a single JSON line.
// The server is single-threaded: it processes one message at a time in the
// order they arrive.
type Server struct {
	tools  *ToolRegistry // registered tools; handles all vault operations
	input  io.Reader     // message source; defaults to os.Stdin
	output io.Writer     // message sink; defaults to os.Stdout
}

// NewServer constructs a [Server] connected to the given vault. The server
// reads from os.Stdin and writes to os.Stdout, which is the standard MCP
// stdio transport expected by editors and AI agent frameworks.
func NewServer(vm *vault.VaultManager, allowWrites bool, auditLog *audit.Log) *Server {
	return &Server{
		tools:  NewToolRegistry(vm, allowWrites, auditLog),
		input:  os.Stdin,
		output: os.Stdout,
	}
}

// Serve enters the main read-dispatch-respond loop. It blocks until stdin is
// closed (EOF) or a scanner error occurs. Blank lines are silently skipped.
// JSON parse errors produce a standard -32700 error response so the client
// can recover rather than stalling.
//
// The scanner buffer is set to 1 MiB to accommodate large tool responses
// (e.g. a vault_health dump for a vault with many secrets). This is well
// above the expected maximum message size but avoids silent truncation.
func (s *Server) Serve() error {
	scanner := bufio.NewScanner(s.input)
	// 1 MiB scanner buffer — large enough for any realistic MCP message while
	// still bounding memory use per request.
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			// Return a parse-error response rather than crashing; the client
			// may send another valid request after receiving the error.
			s.writeError(nil, -32700, "parse error", err.Error())
			continue
		}

		s.handleRequest(&req)
	}

	return scanner.Err()
}

// handleRequest routes an incoming JSON-RPC request to the appropriate
// handler. Notifications (messages without an ID) receive no response; the
// "initialized" notification is the only one Vial currently receives.
func (s *Server) handleRequest(req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized":
		// MCP "initialized" is a client notification sent after the client has
		// processed the initialize response. No reply is expected or sent.
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	case "ping":
		// The ping method is used by some clients for liveness checks.
		// An empty object result is the correct response per the JSON-RPC spec.
		s.writeResult(req.ID, map[string]interface{}{})
	default:
		s.writeError(req.ID, -32601, "method not found", fmt.Sprintf("unknown method: %s", req.Method))
	}
}

// handleInitialize responds to the MCP "initialize" request with the server's
// protocol version, identity, and declared capabilities. This is always the
// first request in an MCP session.
func (s *Server) handleInitialize(req *JSONRPCRequest) {
	result := InitializeResult{
		ProtocolVersion: protocolVersion,
		ServerInfo: ServerInfo{
			Name:    serverName,
			Version: serverVersion,
		},
		Capabilities: Capabilities{
			// Non-nil Tools signals that the tools/* methods are available.
			Tools: &ToolsCapability{},
		},
	}
	s.writeResult(req.ID, result)
}

// handleToolsList responds to "tools/list" with the current tool definitions.
// The list is static for the lifetime of the server.
func (s *Server) handleToolsList(req *JSONRPCRequest) {
	result := ToolsListResult{
		Tools: s.tools.ListTools(),
	}
	s.writeResult(req.ID, result)
}

// handleToolsCall dispatches a "tools/call" request. The raw Params field
// (interface{}) must be round-tripped through JSON to obtain a typed
// [CallToolParams] value, because the JSON decoder stores untyped objects as
// map[string]interface{} when the destination is interface{}.
//
// Tool-level errors (e.g. vault locked, unknown key) are returned inside a
// successful JSON-RPC result with IsError == true. Only protocol-level
// failures (missing or malformed params) produce a JSON-RPC error object.
func (s *Server) handleToolsCall(req *JSONRPCRequest) {
	// Re-marshal and unmarshal Params to get a typed CallToolParams.
	paramsBytes, err := json.Marshal(req.Params)
	if err != nil {
		s.writeError(req.ID, -32602, "invalid params", err.Error())
		return
	}

	var params CallToolParams
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		s.writeError(req.ID, -32602, "invalid params", err.Error())
		return
	}

	result := s.tools.CallTool(params.Name, params.Arguments)
	s.writeResult(req.ID, result)
}

// writeResult sends a successful JSON-RPC 2.0 response. The result value
// is serialised as-is; callers are responsible for passing a JSON-serialisable
// value.
func (s *Server) writeResult(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeResponse(resp)
}

// writeError sends a JSON-RPC 2.0 error response using the provided error
// code, message, and optional detail string. Standard codes are defined in
// [JSONRPCError].
func (s *Server) writeError(id interface{}, code int, message, data string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	s.writeResponse(resp)
}

// writeResponse serialises resp as a single JSON line on s.output. Marshal
// errors are silently dropped — if we cannot serialise the response there is
// nothing useful to write.
func (s *Server) writeResponse(resp JSONRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	// Newline after the JSON object is required by the MCP stdio transport
	// spec so clients can delimit messages by line.
	fmt.Fprintf(s.output, "%s\n", data)
}

// newLockedBuffer wraps raw bytes in a memguard [memguard.LockedBuffer].
// The caller owns the returned buffer and must call Destroy when done.
// Used by the write tools to ensure secret values are handled in mlock'd
// memory before being passed to the vault layer.
func newLockedBuffer(data []byte) *memguard.LockedBuffer {
	return memguard.NewBufferFromBytes(data)
}
