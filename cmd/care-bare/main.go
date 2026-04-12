// Package main is the entry point for the care-bare CLI tool.
// It sets version info from ldflags and delegates to the cli package.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Blue-Bear-Security/care-bare/internal/adapter"
	"github.com/Blue-Bear-Security/care-bare/internal/cli"
)

// Version information injected at build time via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Set the binary path for hook configs so they use the absolute path
	// to this binary, not a PATH-relative name.
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			adapter.BinaryPath = resolved
		} else {
			adapter.BinaryPath = exe
		}
	}

	cli.SetVersionInfo(version, commit, date)
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
