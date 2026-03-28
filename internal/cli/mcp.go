package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/mcp"
)

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

var mcpAllowWrites bool

func init() {
	mcpCmd.Flags().BoolVar(&mcpAllowWrites, "allow-writes", false, "Enable write operations (vault_set, vault_remove)")
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	if mcpAllowWrites {
		fmt.Fprintln(os.Stderr, "vial: MCP server starting (read-write mode)")
	} else {
		fmt.Fprintln(os.Stderr, "vial: MCP server starting (read-only mode)")
	}

	server := mcp.NewServer(vm, mcpAllowWrites)
	return server.Serve()
}
