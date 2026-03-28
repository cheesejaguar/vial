package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/awnumar/memguard"
	"github.com/cheesejaguar/vial/internal/vault"
)

const (
	protocolVersion = "2024-11-05"
	serverName      = "vial"
	serverVersion   = "0.1.0"
)

// Server implements an MCP server using JSON-RPC 2.0 over stdio.
type Server struct {
	tools  *ToolRegistry
	input  io.Reader
	output io.Writer
}

// NewServer creates a new MCP server.
func NewServer(vm *vault.VaultManager, allowWrites bool) *Server {
	return &Server{
		tools:  NewToolRegistry(vm, allowWrites),
		input:  os.Stdin,
		output: os.Stdout,
	}
}

// Serve runs the MCP server, reading JSON-RPC requests from stdin and writing
// responses to stdout. It runs until stdin is closed.
func (s *Server) Serve() error {
	scanner := bufio.NewScanner(s.input)
	// Increase buffer for large messages
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(nil, -32700, "parse error", err.Error())
			continue
		}

		s.handleRequest(&req)
	}

	return scanner.Err()
}

func (s *Server) handleRequest(req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized":
		// Notification — no response needed
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	case "ping":
		s.writeResult(req.ID, map[string]interface{}{})
	default:
		s.writeError(req.ID, -32601, "method not found", fmt.Sprintf("unknown method: %s", req.Method))
	}
}

func (s *Server) handleInitialize(req *JSONRPCRequest) {
	result := InitializeResult{
		ProtocolVersion: protocolVersion,
		ServerInfo: ServerInfo{
			Name:    serverName,
			Version: serverVersion,
		},
		Capabilities: Capabilities{
			Tools: &ToolsCapability{},
		},
	}
	s.writeResult(req.ID, result)
}

func (s *Server) handleToolsList(req *JSONRPCRequest) {
	result := ToolsListResult{
		Tools: s.tools.ListTools(),
	}
	s.writeResult(req.ID, result)
}

func (s *Server) handleToolsCall(req *JSONRPCRequest) {
	// Parse params
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

func (s *Server) writeResult(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeResponse(resp)
}

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

func (s *Server) writeResponse(resp JSONRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	fmt.Fprintf(s.output, "%s\n", data)
}

// newLockedBuffer creates a memguard LockedBuffer from bytes.
func newLockedBuffer(data []byte) *memguard.LockedBuffer {
	return memguard.NewBufferFromBytes(data)
}
