// status.go implements the angry-bear status command.
// It displays enforcement rules, active sessions, discovered skills,
// and detected AI agent integrations for the current project.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blue-Bear-Security/angry-bear/internal/adapter"
	"github.com/Blue-Bear-Security/angry-bear/internal/engine"
	"github.com/Blue-Bear-Security/angry-bear/internal/scanner"
	"github.com/Blue-Bear-Security/angry-bear/internal/state"
	"github.com/spf13/cobra"
)

// NewStatusCommand returns the status subcommand.
// It displays enforcement rules, active sessions with invoked skills,
// discovered skills, and detected AI agent integrations.
func NewStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show enforcement rules and session state",
		Long: `Display the current angry-bear enforcement configuration including:
- Configured enforcement rules and their sources
- Active sessions and their invoked skills
- Discovered skill definitions from configured paths
- Detected AI agent integrations`,
		RunE: runStatus,
	}
	cmd.Flags().String("session", "", "Show details for a specific session")
	return cmd
}

// runStatus is the main handler for the status command. It resolves the
// project root, loads config and state, scans skills, detects agents,
// and prints a structured text report to stdout.
func runStatus(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	sessionFilter, _ := cmd.Flags().GetString("session")

	// Resolve project root from cwd.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	projectRoot := engine.ResolveProjectRoot(cwd)

	// Load and display enforcement rules.
	printEnforcementRules(out, projectRoot)

	// List active sessions from ~/.angry-bear/repos/{hash}/state/.
	stateDir := engine.ResolveStateDir(projectRoot)
	printActiveSessions(out, stateDir, sessionFilter)

	// Discover and display skills from configured paths.
	printDiscoveredSkills(out, projectRoot)

	// Detect and display agent integrations.
	printAgentIntegrations(out, projectRoot)

	return nil
}

// printEnforcementRules loads and displays the merged enforcement config.
// It uses LoadMergedConfig to combine repo-level and machine-level rules,
// matching the resolution order used by the hook and the TUI.
func printEnforcementRules(out io.Writer, projectRoot string) {
	fmt.Fprintln(out, "=== Enforcement Rules ===")

	repoConfigDir := ""
	repo := engine.ResolveRepoIdentity(projectRoot)
	if repo != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr == nil {
			repoConfigDir = engine.RepoConfigDir(home, repo)
		}
	}

	rules, err := engine.LoadMergedConfig(projectRoot, repoConfigDir)
	if err != nil {
		fmt.Fprintf(out, "  Error loading config: %v\n", err)
		fmt.Fprintln(out)
		return
	}

	if len(rules) == 0 {
		fmt.Fprintln(out, "  No enforcement rules configured.")
		fmt.Fprintln(out)
		return
	}

	for i, mr := range rules {
		fmt.Fprintf(out, "  [%d] Tool: %s, Path: %s, Skill: %s, Agent: %s (from %s)\n",
			i+1, mr.Rule.Tool, mr.Rule.Path, mr.Rule.Skill, mr.Rule.Agent, mr.Source)
	}
	fmt.Fprintln(out)
}

// printActiveSessions reads session state files and displays them.
// If sessionFilter is non-empty, only that session is shown.
func printActiveSessions(out io.Writer, stateDir, sessionFilter string) {
	fmt.Fprintln(out, "=== Active Sessions ===")

	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		fmt.Fprintln(out, "  No state directory found.")
		fmt.Fprintln(out)
		return
	}

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		fmt.Fprintf(out, "  Error reading state directory: %v\n", err)
		fmt.Fprintln(out)
		return
	}

	found := false
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		sessionID := strings.TrimSuffix(entry.Name(), ".json")
		if sessionFilter != "" && sessionID != sessionFilter {
			continue
		}

		// Read the session state file.
		data, readErr := os.ReadFile(filepath.Join(stateDir, entry.Name()))
		if readErr != nil {
			fmt.Fprintf(out, "  Session: %s (error reading: %v)\n", sessionID, readErr)
			found = true
			continue
		}

		var ss state.SessionState
		if jsonErr := json.Unmarshal(data, &ss); jsonErr != nil {
			fmt.Fprintf(out, "  Session: %s (corrupt state file)\n", sessionID)
			found = true
			continue
		}

		createdStr := ss.CreatedAt
		if createdStr == "" {
			info, infoErr := entry.Info()
			if infoErr == nil {
				createdStr = info.ModTime().Format("2006-01-02T15:04:05Z07:00")
			}
		}

		fmt.Fprintf(out, "  Session: %s (created %s)\n", sessionID, createdStr)
		if len(ss.InvokedSkills) == 0 {
			fmt.Fprintln(out, "    Invoked skills: (none)")
		} else {
			fmt.Fprintf(out, "    Invoked skills: %s\n", strings.Join(ss.InvokedSkills, ", "))
		}
		found = true
	}

	if !found {
		if sessionFilter != "" {
			fmt.Fprintf(out, "  No session found with ID: %s\n", sessionFilter)
		} else {
			fmt.Fprintln(out, "  No active sessions.")
		}
	}
	fmt.Fprintln(out)
}

// printDiscoveredSkills loads global config, resolves skill paths, scans
// for skills, and displays the results.
func printDiscoveredSkills(out io.Writer, projectRoot string) {
	fmt.Fprintln(out, "=== Discovered Skills ===")

	globalCfg, err := engine.LoadGlobalConfig(projectRoot)
	if err != nil {
		fmt.Fprintf(out, "  Error loading config: %v\n", err)
		fmt.Fprintln(out)
		return
	}

	var skillPaths []string
	for _, sp := range globalCfg.SkillPaths {
		if filepath.IsAbs(sp) {
			skillPaths = append(skillPaths, sp)
		} else {
			skillPaths = append(skillPaths, filepath.Join(projectRoot, sp))
		}
	}

	skills, scanErr := scanner.ScanSkills(skillPaths)
	if scanErr != nil {
		fmt.Fprintf(out, "  Error scanning skills: %v\n", scanErr)
		fmt.Fprintln(out)
		return
	}

	if len(skills) == 0 {
		fmt.Fprintln(out, "  No skills discovered.")
		fmt.Fprintln(out)
		return
	}

	for _, s := range skills {
		fmt.Fprintf(out, "  - %s (%s)\n", s.Name, s.Source)
	}
	fmt.Fprintln(out)
}

// printAgentIntegrations checks the adapter registry for detected agents
// by looking for each agent's marker directory in the project root.
func printAgentIntegrations(out io.Writer, projectRoot string) {
	fmt.Fprintln(out, "=== Agent Integrations ===")

	registry := adapter.NewRegistry()
	anyDetected := false

	for _, name := range registry.Names() {
		hookAdapter, err := registry.Get(name)
		if err != nil {
			continue
		}

		markerDir := filepath.Dir(hookAdapter.ConfigPath())
		markerPath := filepath.Join(projectRoot, markerDir)

		if _, sErr := os.Stat(markerPath); os.IsNotExist(sErr) {
			fmt.Fprintf(out, "  - %s: not detected\n", name)
		} else {
			fmt.Fprintf(out, "  - %s: detected (%s/ exists)\n", name, markerDir)
			anyDetected = true
		}
	}

	if !anyDetected {
		fmt.Fprintln(out, "  No AI agents detected in this project.")
	}
}
