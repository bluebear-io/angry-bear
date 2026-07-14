// resolve.go provides shared config resolution and hook installation helpers
// used across CLI commands and the TUI. These functions ensure consistent
// behavior when locating config files and installing agent hooks.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bluebear-io/angry-bear/internal/adapter"
	"github.com/bluebear-io/angry-bear/internal/engine"
	"github.com/bluebear-io/angry-bear/internal/tui"
)

// ResolveRepoDir returns the repo-keyed config directory for a project.
// Returns ("", nil) if the repo identity can't be resolved.
func ResolveRepoDir(projectRoot string) (string, error) {
	repo := engine.ResolveRepoIdentity(projectRoot)
	if repo == nil {
		return "", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return engine.RepoConfigDir(home, repo), nil
}

// ResolveConfigForProject determines the config file path for a given project
// root directory. It checks repo-keyed config dir first (~/.angry-bear/repos/{hash}/),
// then falls back to project-level ({projectRoot}/.angry-bear/).
//
// This is the canonical way to find the config file for any project root and
// should be used by all commands that need to read or write config.
func ResolveConfigForProject(projectRoot string) (string, error) {
	repo := engine.ResolveRepoIdentity(projectRoot)
	if repo != nil {
		home, err := os.UserHomeDir()
		if err == nil {
			repoConfigDir := engine.RepoConfigDir(home, repo)
			return filepath.Join(repoConfigDir, "skill_enforcement.json"), nil
		}
	}

	// Fall back to project-level config.
	return filepath.Join(projectRoot, ".angry-bear", "skill_enforcement.json"), nil
}

// HookSetupResult reports what happened during hook installation.
type HookSetupResult struct {
	Installed []string // Agents where hooks were newly installed
	Skipped   []string // Agents not present on this machine
	Warnings  []string // Errors during installation
}

// EnsureHooksInstalled checks if angry-bear hooks are installed in agent configs.
// If not, installs them and reports what happened. Idempotent — safe to call
// on every CLI invocation.
func EnsureHooksInstalled() HookSetupResult {
	var result HookSetupResult
	registry := adapter.NewRegistry()
	home, err := os.UserHomeDir()
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("cannot determine home directory: %v", err))
		return result
	}

	for _, name := range registry.Names() {
		a, err := registry.Get(name)
		if err != nil {
			continue
		}
		// Check if agent is present on this machine.
		configDir := filepath.Join(home, "."+name)
		if _, err := os.Stat(configDir); err != nil {
			result.Skipped = append(result.Skipped, name)
			continue
		}
		// Check if already installed by reading the config file
		alreadyInstalled := hookAlreadyInstalled(a, home)

		// Install hook (idempotent — safe to call again).
		if err := a.InstallHook(""); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to install hook for %s: %v", name, err))
		} else if !alreadyInstalled {
			result.Installed = append(result.Installed, name)
		}
	}
	return result
}

// hookAlreadyInstalled checks if angry-bear hook is already in the agent's config.
func hookAlreadyInstalled(a adapter.HookAdapter, _ string) bool {
	data, err := os.ReadFile(a.GlobalConfigPath())
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "angry-bear hook")
}

// PrintHookSetup logs hook installation results to stderr on first run.
// Only prints if hooks were newly installed or if there were warnings.
func PrintHookSetup(r HookSetupResult) {
	for _, w := range r.Warnings {
		fmt.Fprintf(os.Stderr, "  \033[38;5;204m!\033[0m %s\n", w)
	}
	if len(r.Installed) > 0 {
		for _, name := range r.Installed {
			fmt.Fprintf(os.Stderr, "  \033[38;5;34m\u2713\033[0m Hooks installed for %s\n", name)
		}
		fmt.Fprintln(os.Stderr)
	}
}

// CheckHookHealth returns a health map for all registered adapters.
// Uses the adapter registry — new adapters automatically get checked.
func CheckHookHealth() map[string]bool {
	health := make(map[string]bool)
	registry := adapter.NewRegistry()
	for _, name := range registry.Names() {
		a, err := registry.Get(name)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(a.GlobalConfigPath())
		if err != nil {
			continue
		}
		health[name] = strings.Contains(string(data), "angry-bear hook")
	}
	return health
}

// InitAgentOptions sets tui.AgentOptions from the adapter registry.
// Called at startup so the TUI knows about all registered agents.
func InitAgentOptions() {
	registry := adapter.NewRegistry()
	names := registry.Names()
	tui.AgentOptions = append(names, "*")
}
