// Package main is the entry point for the angry-bear CLI tool.
// It sets version info from ldflags and delegates to the cli package.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Blue-Bear-Security/angry-bear/internal/adapter"
	"github.com/Blue-Bear-Security/angry-bear/internal/cli"
)

// Version information injected at build time via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Set the binary path for hook configs so they use the absolute path
	// to this binary, not a PATH-relative name. This propagates to all
	// adapters created by NewRegistry.
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			adapter.SetRegistryDefaults("", resolved)
		} else {
			adapter.SetRegistryDefaults("", exe)
		}
	}

	cli.SetVersionInfo(version, commit, date)
	if err := cli.Execute(); err != nil {
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
