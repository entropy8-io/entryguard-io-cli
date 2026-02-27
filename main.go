package main

import (
	"os"

	"github.com/entryguard-io/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
