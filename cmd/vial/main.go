package main

import (
	"os"

	"github.com/awnumar/memguard"
	"github.com/cheesejaguar/vial/internal/cli"
)

func main() {
	memguard.CatchInterrupt()
	defer memguard.Purge()

	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
