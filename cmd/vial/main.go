// Package main is the entry point for the vial command-line tool.
//
// It initialises memguard for secure memory handling before delegating to the
// CLI package. memguard.CatchInterrupt ensures that all mlock'd secret buffers
// are zeroed and freed even when the process receives SIGINT or SIGTERM;
// the deferred memguard.Purge provides the same guarantee on a clean exit.
// Secret values are never held in ordinary heap allocations after this point —
// all sensitive material flows through memguard.LockedBuffer throughout the
// rest of the program.
package main

import (
	"fmt"
	"os"

	"github.com/awnumar/memguard"
	"github.com/cheesejaguar/vial/internal/cli"
)

func main() {
	// Register a signal handler so that an interrupt (^C, SIGTERM, etc.)
	// triggers a full memguard purge before the process exits. Without this,
	// mlock'd pages containing key material could be left in memory if the
	// user kills the process mid-operation.
	memguard.CatchInterrupt()

	// Purge all LockedBuffers on any normal return path. This is the last-
	// resort cleanup; individual commands are responsible for calling
	// Destroy() on buffers they own as soon as those buffers are no longer
	// needed.
	defer memguard.Purge()

	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
