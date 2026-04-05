package cli

import (
	"fmt"
	"os"

	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/audit"
	"github.com/cheesejaguar/vial/internal/mcp"
)

// mcpCmd starts the Model Context Protocol server that exposes vault
// operations to AI coding tools (Claude Code, Cursor, etc.) over JSON-RPC 2.0
// via stdin/stdout. The server runs until the process is killed; it does not
// exit on its own.
//
// By default the server is read-only to limit blast radius from a compromised
// AI session. Pass --allow-writes to enable vault_set and vault_remove.
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for AI coding tools",
	Long: `Start a Model Context Protocol (MCP) server that exposes vault operations
to AI coding tools like Claude Code, Cursor, and other MCP-compatible clients.

The server communicates via JSON-RPC 2.0 over stdio (stdin/stdout).

Available tools:
  vault_list    — List all secret key names
  vault_get     — Get a secret value by key
  vault_search  — Search secrets by name pattern
  vault_health  — Get health status of all secrets

With --allow-writes:
  vault_set     — Store a secret
  vault_remove  — Remove a secret

Configure in your MCP client:
  {
    "mcpServers": {
      "vial": {
        "command": "vial",
        "args": ["mcp"]
      }
    }
  }`,
	RunE: runMCP,
}

// mcpAllowWrites gates write operations in the MCP server. When false (the
// default), vault_set and vault_remove are not registered as available tools,
// so AI clients cannot modify the vault even if they request it.
var mcpAllowWrites bool

func init() {
	mcpCmd.Flags().BoolVar(&mcpAllowWrites, "allow-writes", false, "Enable write operations (vault_set, vault_remove)")
	rootCmd.AddCommand(mcpCmd)
}

// runMCP handles the mcp command. Status messages are written to stderr so
// that stdout stays clean for the JSON-RPC 2.0 framing that MCP clients read.
// The audit log is co-located with the vault file and records every tool call
// made through the MCP interface.
func runMCP(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	// Lock is deferred so the DEK is wiped from mlock'd memory when the
	// server exits, even on panics or OS signals caught by Go's runtime.
	defer vm.Lock()

	// Announce mode on stderr before handing stdout to the JSON-RPC framer.
	if mcpAllowWrites {
		fmt.Fprintln(os.Stderr, "vial: MCP server starting (read-write mode)")
	} else {
		fmt.Fprintln(os.Stderr, "vial: MCP server starting (read-only mode)")
	}

	if err := loadConfig(); err != nil {
		return err
	}

	// Place audit.jsonl alongside the vault file so all vial data stays in
	// one directory (~/.local/share/vial/).
	auditPath := filepath.Join(filepath.Dir(cfg.VaultPath), "audit.jsonl")
	auditLog := audit.NewLog(auditPath)

	server := mcp.NewServer(vm, mcpAllowWrites, auditLog)
	return server.Serve()
}
