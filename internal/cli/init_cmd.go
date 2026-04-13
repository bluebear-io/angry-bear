// init_cmd.go implements the care-bare init command for project setup.
// It creates the .care-bare/ directory structure, writes default configuration files,
// detects which AI agents are present, installs hooks via the adapter layer,
// and updates .gitignore to exclude transient state files.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Blue-Bear-Security/care-bare/internal/adapter"
	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/spf13/cobra"
)

// execLookPath wraps exec.LookPath for testability.
var execLookPath = exec.LookPath

// NewInitCommand returns the init subcommand that bootstraps a project for care-bare.
func NewInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize care-bare in the current project",
		Long: `Initialize care-bare in the current project directory.

This command creates the .care-bare/ directory structure, writes default
configuration files, detects AI agents (Claude, Cursor), installs hooks
into their respective configuration files, and updates .gitignore to
exclude transient state files.

Safe to run multiple times -- existing configs are preserved and hooks
are not duplicated.`,
		RunE: runInit,
	}
}

// runInit is the main init handler that orchestrates project bootstrapping.
func runInit(cmd *cobra.Command, args []string) error {
	// Step 1: Determine the project directory from cwd.
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("determining working directory: %w", err)
	}

	out := cmd.OutOrStdout()

	// Track what was created vs preserved for the summary.
	var created []string
	var preserved []string
	var agentResults []string
	var updated []string

	// Step 2: Create .care-bare/ directory.
	careBareDir := filepath.Join(projectDir, ".care-bare")
	if err := os.MkdirAll(careBareDir, 0o755); err != nil {
		return fmt.Errorf("creating .care-bare directory: %w", err)
	}
	created = append(created, ".care-bare/")

	// Step 3: Create .care-bare/state/ directory.
	stateDir := filepath.Join(careBareDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("creating .care-bare/state directory: %w", err)
	}
	created = append(created, ".care-bare/state/")

	// Step 4: Write default skill_enforcement.json (only if not exists).
	enforcementPath := filepath.Join(careBareDir, "skill_enforcement.json")
	if _, err := os.Stat(enforcementPath); os.IsNotExist(err) {
		defaultConfig := engine.Config{
			Version: 1,
			Tools:   []engine.Rule{},
		}
		data, err := json.MarshalIndent(defaultConfig, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling skill_enforcement.json: %w", err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(enforcementPath, data, 0o644); err != nil {
			return fmt.Errorf("writing skill_enforcement.json: %w", err)
		}
		created = append(created, ".care-bare/skill_enforcement.json")
	} else if err == nil {
		preserved = append(preserved, ".care-bare/skill_enforcement.json")
	} else {
		return fmt.Errorf("checking skill_enforcement.json: %w", err)
	}

	// Step 5: Write default config.json (only if not exists).
	configPath := filepath.Join(careBareDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultGlobal := engine.GlobalConfig{
			SkillPaths:    []string{".claude/skills"},
			StateTTLHours: 24,
			DefaultAgent:  "*",
			IgnorePatterns: []string{
				".git", "node_modules", "vendor", "dist",
				".next", "__pycache__", ".venv", "build", "target",
			},
		}
		data, err := json.MarshalIndent(defaultGlobal, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling config.json: %w", err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(configPath, data, 0o644); err != nil {
			return fmt.Errorf("writing config.json: %w", err)
		}
		created = append(created, ".care-bare/config.json")
	} else if err == nil {
		preserved = append(preserved, ".care-bare/config.json")
	} else {
		return fmt.Errorf("checking config.json: %w", err)
	}

	// Step 6: Detect AI agents and install hooks.
	registry := adapter.NewRegistry()
	var hookErrors []error

	for _, name := range registry.Names() {
		hookAdapter, err := registry.Get(name)
		if err != nil {
			continue
		}

		// Derive marker directory from the adapter's config path.
		// e.g., ".claude/settings.json" -> ".claude"
		markerDir := filepath.Dir(hookAdapter.ConfigPath())
		markerPath := filepath.Join(projectDir, markerDir)

		if _, err := os.Stat(markerPath); os.IsNotExist(err) {
			continue
		}

		// Agent detected -- install hook.
		if err := hookAdapter.InstallHook(projectDir); err != nil {
			hookErrors = append(hookErrors, fmt.Errorf("%s: %w", name, err))
			agentResults = append(agentResults, fmt.Sprintf("  %s - hook installation failed: %v", name, err))
		} else {
			agentResults = append(agentResults, fmt.Sprintf("  %s - hook installed in %s", name, hookAdapter.ConfigPath()))
		}
	}

	// Step 7: Update .gitignore.
	gitignoreErr := updateGitignore(projectDir)
	if gitignoreErr == nil {
		updated = append(updated, ".gitignore - added .care-bare/state/")
	}

	// Step 8: Print summary.
	fmt.Fprintf(out, "care-bare initialized in %s\n", projectDir)

	if len(created) > 0 {
		fmt.Fprintln(out, "\nCreated:")
		for _, item := range created {
			fmt.Fprintf(out, "  %s\n", item)
		}
	}

	if len(preserved) > 0 {
		fmt.Fprintln(out, "\nPreserved (already existed):")
		for _, item := range preserved {
			fmt.Fprintf(out, "  %s\n", item)
		}
	}

	if len(agentResults) > 0 {
		fmt.Fprintln(out, "\nAI Agents Detected:")
		for _, result := range agentResults {
			fmt.Fprintln(out, result)
		}
	} else {
		fmt.Fprintln(out, "\nNo AI agents detected. Install hooks manually with: care-bare init --agent <name>")
	}

	if len(updated) > 0 {
		fmt.Fprintln(out, "\nUpdated:")
		for _, item := range updated {
			fmt.Fprintf(out, "  %s\n", item)
		}
	}

	// Check if care-bare is accessible by name on PATH.
	_, lookupErr := execLookPath("care-bare")
	if lookupErr != nil {
		fmt.Fprintln(out, "\nSetup:")
		fmt.Fprintln(out, "  care-bare is not on your PATH. To fix this, run ONE of:")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "    # Install globally via Go")
		fmt.Fprintln(out, "    cd "+filepath.Dir(filepath.Dir(adapter.RegistryBinaryPath()))+" && make install")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "    # Or install via Homebrew (once published)")
		fmt.Fprintln(out, "    brew install Blue-Bear-Security/tap/care-bare")
		fmt.Fprintln(out, "")
	}

	fmt.Fprintln(out, "Next steps:")
	fmt.Fprintln(out, "  1. Add enforcement rules: care-bare   (launches TUI)")
	fmt.Fprintln(out, "  2. Check installation: care-bare doctor")

	// Return error if any hook installations failed.
	if len(hookErrors) > 0 {
		return fmt.Errorf("hook installation failed for %d agent(s)", len(hookErrors))
	}

	return nil
}

// updateGitignore appends .care-bare/state/ to the project's .gitignore
// if it is not already present. Creates .gitignore if it does not exist.
func updateGitignore(projectDir string) error {
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	entry := ".care-bare/state/"

	// Read existing .gitignore content (may not exist).
	var content []byte
	existingData, err := os.ReadFile(gitignorePath)
	if err == nil {
		content = existingData
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}

	// Check if the entry already exists by scanning lines.
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == ".care-bare/state/" || trimmed == ".care-bare/state" {
			return nil // Already present, nothing to do.
		}
	}

	// Append the entry. Ensure the file ends with a newline before our addition.
	var builder strings.Builder
	builder.Write(content)
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("# care-bare state (generated, do not commit)\n")
	builder.WriteString(entry + "\n")

	if err := os.WriteFile(gitignorePath, []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	return nil
}
