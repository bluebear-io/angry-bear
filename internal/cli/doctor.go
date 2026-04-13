// doctor.go implements the care-bare doctor command for installation diagnostics.
// It runs a series of health checks and reports pass/fail status with fix
// suggestions for any failures. Exits with code 1 if any check fails.
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
	"github.com/Blue-Bear-Security/care-bare/internal/scanner"
	"github.com/spf13/cobra"
)

// checkResult represents the outcome of a single diagnostic check.
type checkResult struct {
	Name    string // e.g., "Config validity: skill_enforcement.json"
	Passed  bool
	Detail  string // e.g., "version 1, 3 rules"
	FixHint string // e.g., "Run 'care-bare add'..."
}

// NewDoctorCommand returns the doctor subcommand.
// It validates the health of the care-bare installation with a pass/fail
// checklist and actionable fix suggestions.
func NewDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check care-bare installation health",
		Long: `Validate the health of the care-bare installation.

Runs diagnostic checks on:
- Config file validity
- Hook installation for detected agents
- State directory existence and writability
- Binary availability on PATH
- Skill path existence and contents

Exits with code 1 if any check fails.`,
		RunE: runDoctor,
	}
}

// runDoctor is the main handler for the doctor command. It resolves the
// project root, runs all diagnostic checks, and prints a summary.
func runDoctor(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	// Resolve project root from cwd.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	projectRoot := engine.ResolveProjectRoot(cwd)

	fmt.Fprintln(out, "care-bare doctor")
	fmt.Fprintln(out, "================")
	fmt.Fprintln(out)

	// Collect all check results.
	var results []checkResult

	results = append(results, checkConfigValidity(projectRoot)...)
	results = append(results, checkHookInstallation(projectRoot)...)
	results = append(results, checkStateDirectory(projectRoot))
	results = append(results, checkBinaryOnPath())
	results = append(results, checkSkillPaths(projectRoot)...)

	// Print results.
	passed := 0
	total := len(results)
	for _, r := range results {
		if r.Passed {
			passed++
			detail := ""
			if r.Detail != "" {
				detail = " - " + r.Detail
			}
			fmt.Fprintf(out, "[PASS] %s%s\n", r.Name, detail)
		} else {
			detail := ""
			if r.Detail != "" {
				detail = " - " + r.Detail
			}
			fmt.Fprintf(out, "[FAIL] %s%s\n", r.Name, detail)
			if r.FixHint != "" {
				fmt.Fprintf(out, "       Fix: %s\n", r.FixHint)
			}
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "Result: %d/%d checks passed\n", passed, total)

	if passed < total {
		// Return a sentinel error so Cobra sets exit code 1.
		return fmt.Errorf("%d check(s) failed", total-passed)
	}
	return nil
}

// checkConfigValidity validates skill_enforcement.json and config.json.
// Returns one or two check results depending on what files exist.
func checkConfigValidity(projectRoot string) []checkResult {
	var results []checkResult

	// Check skill_enforcement.json.
	enforcementPath := filepath.Join(projectRoot, ".care-bare", "skill_enforcement.json")
	results = append(results, checkEnforcementConfig(enforcementPath))

	// Check config.json.
	configPath := filepath.Join(projectRoot, ".care-bare", "config.json")
	results = append(results, checkGlobalConfig(configPath))

	return results
}

// checkEnforcementConfig parses the enforcement config file and returns a check result.
func checkEnforcementConfig(path string) checkResult {
	name := "Config validity: skill_enforcement.json"

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				Name:   name,
				Passed: true,
				Detail: "not present (no rules enforced)",
			}
		}
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("cannot read: %v", err),
			FixHint: "Check file permissions on .care-bare/skill_enforcement.json.",
		}
	}

	var cfg engine.Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("invalid JSON: %v", err),
			FixHint: "Fix the JSON syntax in .care-bare/skill_enforcement.json.",
		}
	}

	if cfg.Version != 1 {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("unsupported version %d", cfg.Version),
			FixHint: "Set \"version\": 1 in skill_enforcement.json.",
		}
	}

	return checkResult{
		Name:   name,
		Passed: true,
		Detail: fmt.Sprintf("version %d, %d rules", cfg.Version, len(cfg.Tools)),
	}
}

// checkGlobalConfig parses config.json and returns a check result.
func checkGlobalConfig(path string) checkResult {
	name := "Config validity: config.json"

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				Name:   name,
				Passed: true,
				Detail: "not present (defaults used)",
			}
		}
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("cannot read: %v", err),
			FixHint: "Check file permissions on .care-bare/config.json.",
		}
	}

	var cfg engine.GlobalConfig
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("invalid JSON: %v", err),
			FixHint: "Fix the JSON syntax in .care-bare/config.json.",
		}
	}

	return checkResult{
		Name:   name,
		Passed: true,
		Detail: "valid",
	}
}

// checkHookInstallation verifies that care-bare hooks are installed for
// each detected agent. Returns one check result per detected agent.
func checkHookInstallation(projectRoot string) []checkResult {
	var results []checkResult

	registry := adapter.NewRegistry()
	for _, agentName := range registry.Names() {
		hookAdapter, err := registry.Get(agentName)
		if err != nil {
			continue
		}

		markerDir := filepath.Dir(hookAdapter.ConfigPath())
		markerPath := filepath.Join(projectRoot, markerDir)

		// Only check agents that are detected (directory exists).
		if _, sErr := os.Stat(markerPath); os.IsNotExist(sErr) {
			continue
		}

		result := checkAgentHook(projectRoot, agentName, hookAdapter)
		results = append(results, result)
	}

	return results
}

// checkAgentHook reads the agent's config file and checks for a care-bare hook entry.
func checkAgentHook(projectRoot, agentName string, hookAdapter adapter.HookAdapter) checkResult {
	name := fmt.Sprintf("Hook installed: %s (%s)", agentName, hookAdapter.ConfigPath())

	configPath := filepath.Join(projectRoot, hookAdapter.ConfigPath())
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				Name:    name,
				Passed:  false,
				Detail:  "config file not found",
				FixHint: fmt.Sprintf("Run 'care-bare add' to install hooks for %s.", agentName),
			}
		}
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("cannot read config: %v", err),
			FixHint: fmt.Sprintf("Check file permissions on %s.", hookAdapter.ConfigPath()),
		}
	}

	if !strings.Contains(string(data), "care-bare hook") {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  "hook entry not found",
			FixHint: fmt.Sprintf("Run 'care-bare add' to install hooks for %s.", agentName),
		}
	}

	return checkResult{
		Name:   name,
		Passed: true,
	}
}

// checkStateDirectory verifies that .care-bare/state/ exists and is writable.
func checkStateDirectory(projectRoot string) checkResult {
	name := "State directory: .care-bare/state/"
	stateDir := filepath.Join(projectRoot, ".care-bare", "state")

	info, err := os.Stat(stateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				Name:    name,
				Passed:  false,
				Detail:  "does not exist",
				FixHint: "Run 'care-bare add' to create the state directory.",
			}
		}
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("cannot stat: %v", err),
			FixHint: "Check directory permissions on .care-bare/state/.",
		}
	}

	if !info.IsDir() {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  "exists but is not a directory",
			FixHint: "Remove .care-bare/state and run 'care-bare add'.",
		}
	}

	// Check writability by creating and removing a temp file.
	tmpPath := filepath.Join(stateDir, ".doctor-write-test")
	err = os.WriteFile(tmpPath, []byte("test"), 0o600)
	if err != nil {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  "exists but is not writable",
			FixHint: "Check directory permissions on .care-bare/state/.",
		}
	}
	os.Remove(tmpPath)

	return checkResult{
		Name:   name,
		Passed: true,
		Detail: "exists and is writable",
	}
}

// checkBinaryOnPath uses exec.LookPath to verify care-bare is on the system PATH.
func checkBinaryOnPath() checkResult {
	name := "Binary on PATH"

	path, err := exec.LookPath("care-bare")
	if err != nil {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  "care-bare not found on PATH",
			FixHint: "Add care-bare to your PATH or install via 'brew install blue-bear-security/tap/care-bare'.",
		}
	}

	return checkResult{
		Name:   name,
		Passed: true,
		Detail: fmt.Sprintf("found at %s", path),
	}
}

// checkSkillPaths loads the global config and checks each configured skill path
// for existence and discoverable skill files.
func checkSkillPaths(projectRoot string) []checkResult {
	var results []checkResult

	globalCfg, err := engine.LoadGlobalConfig(projectRoot)
	if err != nil {
		results = append(results, checkResult{
			Name:    "Skill paths",
			Passed:  false,
			Detail:  fmt.Sprintf("cannot load config: %v", err),
			FixHint: "Fix .care-bare/config.json.",
		})
		return results
	}

	for _, sp := range globalCfg.SkillPaths {
		var absPath string
		if filepath.IsAbs(sp) {
			absPath = sp
		} else {
			absPath = filepath.Join(projectRoot, sp)
		}

		result := checkSingleSkillPath(sp, absPath)
		results = append(results, result)
	}

	return results
}

// checkSingleSkillPath verifies that a skill path exists and contains
// discoverable skill files.
func checkSingleSkillPath(displayPath, absPath string) checkResult {
	name := fmt.Sprintf("Skill path: %s", displayPath)

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  "does not exist",
			FixHint: fmt.Sprintf("Skill path '%s' does not exist or contains no skill files.", displayPath),
		}
	}

	// Scan for skills in this single path.
	skills, err := scanner.ScanSkills([]string{absPath})
	if err != nil {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("scan error: %v", err),
			FixHint: fmt.Sprintf("Check permissions on '%s'.", displayPath),
		}
	}

	if len(skills) == 0 {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  "no skill files found",
			FixHint: fmt.Sprintf("Skill path '%s' does not exist or contains no skill files.", displayPath),
		}
	}

	return checkResult{
		Name:   name,
		Passed: true,
		Detail: fmt.Sprintf("found %d skills", len(skills)),
	}
}
