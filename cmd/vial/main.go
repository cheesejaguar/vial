package main

import (
	"fmt"
	"os"

	"github.com/awnumar/memguard"
	"github.com/cheesejaguar/vial/internal/cli"
)

func main() {
	memguard.CatchInterrupt()
	defer memguard.Purge()

	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
