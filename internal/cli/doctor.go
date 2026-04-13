// doctor.go implements the care-bear doctor command for installation diagnostics.
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

	"github.com/Blue-Bear-Security/care-bear/internal/adapter"
	"github.com/Blue-Bear-Security/care-bear/internal/engine"
	"github.com/Blue-Bear-Security/care-bear/internal/scanner"
	"github.com/spf13/cobra"
)

// checkResult represents the outcome of a single diagnostic check.
type checkResult struct {
	Name    string // e.g., "Config validity: skill_enforcement.json"
	Passed  bool
	Detail  string // e.g., "version 1, 3 rules"
	FixHint string // e.g., "Run 'care-bear add'..."
}

// NewDoctorCommand returns the doctor subcommand.
// It validates the health of the care-bear installation with a pass/fail
// checklist and actionable fix suggestions.
func NewDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check care-bear installation health",
		Long: `Validate the health of the care-bear installation.

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

	fmt.Fprintln(out, "care-bear doctor")
	fmt.Fprintln(out, "================")
	fmt.Fprintln(out)

	// Collect all check results.
	var results []checkResult

	results = append(results, checkConfigValidity(projectRoot)...)
	results = append(results, checkHookInstallation(projectRoot)...)
	results = append(results, checkStateDirectory(projectRoot))
	results = append(results, checkBinaryOnPath())
	results = append(results, checkSkillPaths(projectRoot, cwd)...)

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
// Uses ResolveConfigForProject to find the enforcement config in the same
// location as the hook (repo-keyed dir first, project-level fallback).
func checkConfigValidity(projectRoot string) []checkResult {
	var results []checkResult

	// Check skill_enforcement.json via the shared resolver so we look in the
	// same repo-keyed directory (~/.care-bear/repos/{hash}/) that the hook uses.
	enforcementPath, err := ResolveConfigForProject(projectRoot)
	if err != nil {
		results = append(results, checkResult{
			Name:    "Config validity: skill_enforcement.json",
			Passed:  false,
			Detail:  fmt.Sprintf("cannot resolve config path: %v", err),
			FixHint: "Check home directory permissions.",
		})
	} else {
		results = append(results, checkEnforcementConfig(enforcementPath))
	}

	// Check config.json.
	configPath := filepath.Join(projectRoot, ".care-bear", "config.json")
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
			FixHint: "Check file permissions on .care-bear/skill_enforcement.json.",
		}
	}

	var cfg engine.Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("invalid JSON: %v", err),
			FixHint: "Fix the JSON syntax in .care-bear/skill_enforcement.json.",
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
			FixHint: "Check file permissions on .care-bear/config.json.",
		}
	}

	var cfg engine.GlobalConfig
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("invalid JSON: %v", err),
			FixHint: "Fix the JSON syntax in .care-bear/config.json.",
		}
	}

	return checkResult{
		Name:   name,
		Passed: true,
		Detail: "valid",
	}
}

// checkHookInstallation verifies that care-bear hooks are installed for
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

// checkAgentHook reads the agent's config file and checks for a care-bear hook entry.
func checkAgentHook(projectRoot, agentName string, hookAdapter adapter.HookAdapter) checkResult {
	name := fmt.Sprintf("Hook installed: %s (%s)", agentName, hookAdapter.ConfigPath())

	configPath := hookAdapter.GlobalConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				Name:    name,
				Passed:  false,
				Detail:  "config file not found",
				FixHint: fmt.Sprintf("Run 'care-bear add' to install hooks for %s.", agentName),
			}
		}
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("cannot read config: %v", err),
			FixHint: fmt.Sprintf("Check file permissions on %s.", hookAdapter.ConfigPath()),
		}
	}

	if !strings.Contains(string(data), "care-bear hook") {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  "hook entry not found",
			FixHint: fmt.Sprintf("Run 'care-bear add' to install hooks for %s.", agentName),
		}
	}

	return checkResult{
		Name:   name,
		Passed: true,
	}
}

// checkStateDirectory verifies that .care-bear/state/ exists and is writable.
// A missing state directory is not a failure because it is created lazily on
// first hook invocation. It is reported as PASS with an informational note.
func checkStateDirectory(projectRoot string) checkResult {
	name := "State directory: .care-bear/state/"
	stateDir := filepath.Join(projectRoot, ".care-bear", "state")

	info, err := os.Stat(stateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				Name:   name,
				Passed: true,
				Detail: "not yet created (will be created on first hook invocation)",
			}
		}
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  fmt.Sprintf("cannot stat: %v", err),
			FixHint: "Check directory permissions on .care-bear/state/.",
		}
	}

	if !info.IsDir() {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  "exists but is not a directory",
			FixHint: "Remove .care-bear/state and run 'care-bear add'.",
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
			FixHint: "Check directory permissions on .care-bear/state/.",
		}
	}
	os.Remove(tmpPath)

	return checkResult{
		Name:   name,
		Passed: true,
		Detail: "exists and is writable",
	}
}

// checkBinaryOnPath uses exec.LookPath to verify care-bear is on the system PATH.
func checkBinaryOnPath() checkResult {
	name := "Binary on PATH"

	path, err := exec.LookPath("care-bear")
	if err != nil {
		return checkResult{
			Name:    name,
			Passed:  false,
			Detail:  "care-bear not found on PATH",
			FixHint: "Add care-bear to your PATH or install via 'brew install blue-bear-security/tap/care-bear'.",
		}
	}

	return checkResult{
		Name:   name,
		Passed: true,
		Detail: fmt.Sprintf("found at %s", path),
	}
}

// checkSkillPaths loads the global config and checks each configured skill path
// for existence and discoverable skill files. Relative skill paths are resolved
// against cwd (the actual project directory) rather than projectRoot, because
// skills are project-level and projectRoot may resolve to the home directory
// when ~/.care-bear/ exists.
func checkSkillPaths(projectRoot, cwd string) []checkResult {
	var results []checkResult

	globalCfg, err := engine.LoadGlobalConfig(projectRoot)
	if err != nil {
		results = append(results, checkResult{
			Name:    "Skill paths",
			Passed:  false,
			Detail:  fmt.Sprintf("cannot load config: %v", err),
			FixHint: "Fix .care-bear/config.json.",
		})
		return results
	}

	for _, sp := range globalCfg.SkillPaths {
		var absPath string
		if filepath.IsAbs(sp) {
			absPath = sp
		} else {
			absPath = filepath.Join(cwd, sp)
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
