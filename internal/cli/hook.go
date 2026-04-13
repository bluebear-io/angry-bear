// hook.go implements the care-bear hook command for PreToolUse enforcement.
// It is invoked by AI agents on every tool use as a pre-tool-use hook.
// The command reads JSON from stdin, determines whether the operation should
// be allowed or blocked based on enforcement rules and loaded skills, and
// writes the appropriate response to stdout.
package cli

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blue-Bear-Security/care-bear/internal/adapter"
	"github.com/Blue-Bear-Security/care-bear/internal/engine"
	"github.com/Blue-Bear-Security/care-bear/internal/state"
	"github.com/spf13/cobra"
)

// maxStdinSize is the maximum number of bytes read from stdin (5MB).
// Anything beyond this is likely malformed or adversarial input.
const maxStdinSize = 5 * 1024 * 1024

// ExitError is returned when the hook needs the process to exit with a
// specific code (e.g., exit code 2 to signal a block to Cursor).
// The caller in main.go checks for this error and calls os.Exit(e.Code).
type ExitError struct {
	Code int
}

// Error returns a human-readable description of the exit code request.
func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

// NewHookCommand returns the hook subcommand that runs as a PreToolUse hook
// for AI agents. It reads JSON from stdin, evaluates enforcement rules, and
// writes allow/deny JSON to stdout.
func NewHookCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Run as a PreToolUse hook for AI agents",
		RunE:  runHook,
	}
	cmd.Flags().String("agent", "", "Agent adapter to use (claude, cursor)")
	return cmd
}

// runHook is the main hook handler that orchestrates the enforcement flow.
// It reads stdin, selects an adapter, parses the input, checks enforcement
// rules, and writes the appropriate response to stdout.
func runHook(cmd *cobra.Command, args []string) error {
	// Determine verbosity from the root command's --verbose flag.
	verbose, _ := cmd.Flags().GetBool("verbose")
	logLevel := slog.LevelWarn
	if verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: logLevel}))

	// Step 1: Read stdin with size limit.
	stdinReader := cmd.InOrStdin()
	limitedReader := io.LimitReader(stdinReader, maxStdinSize+1)
	stdinBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	if len(stdinBytes) > maxStdinSize {
		return fmt.Errorf("stdin exceeds maximum size of %d bytes", maxStdinSize)
	}
	logger.Debug("read stdin", "bytes", len(stdinBytes))

	// Step 2: Select adapter.
	agentFlag, _ := cmd.Flags().GetString("agent")
	registry := adapter.NewRegistry()

	var hookAdapter adapter.HookAdapter
	if agentFlag != "" {
		hookAdapter, err = registry.Get(agentFlag)
		if err != nil {
			return fmt.Errorf("selecting adapter: %w", err)
		}
		logger.Debug("adapter selected via --agent flag", "adapter", hookAdapter.Name())
	} else {
		hookAdapter, err = registry.AutoDetect(stdinBytes)
		if err != nil {
			return fmt.Errorf("auto-detecting adapter: %w", err)
		}
		logger.Debug("adapter auto-detected", "adapter", hookAdapter.Name())
	}

	// Step 3: Parse input.
	hookInput, err := hookAdapter.ParseInput(bytes.NewReader(stdinBytes))
	if err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}
	logger.Debug("parsed input",
		"session_id", hookInput.SessionID,
		"tool_name", hookInput.ToolName,
		"file_path", hookInput.FilePath,
		"cwd", hookInput.Cwd,
	)

	// Step 4: Resolve project root and repo identity.
	projectRoot := engine.ResolveProjectRoot(hookInput.Cwd)
	logger.Debug("resolved project root", "root", projectRoot)

	repo := engine.ResolveRepoIdentity(projectRoot)
	repoConfigDir := ""
	if repo != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			logger.Warn("failed to get home directory for repo config", "error", err)
		} else {
			repoConfigDir = engine.RepoConfigDir(home, repo)
			if err := os.MkdirAll(repoConfigDir, 0o755); err != nil {
				logger.Warn("failed to create repo config directory", "path", repoConfigDir, "error", err)
			}
			logger.Debug("resolved repo identity", "slug", repo.Slug, "configDir", repoConfigDir)
		}
	}

	// Step 5: Check for skill invocation (short-circuit).
	skillName, isSkill := hookAdapter.DetectSkillInvocation(hookInput)
	if isSkill {
		logger.Debug("skill invocation detected", "skill", skillName)
		logSkillEvent(projectRoot, hookInput, skillName, "invoke")
		stateDir := filepath.Join(projectRoot, ".care-bear", "state")
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			logger.Warn("failed to create state directory", "error", err)
			// Fail open: allow the skill invocation even if we can't record it.
			return writeAllow(cmd, hookAdapter)
		}
		mgr := state.NewStateManager(stateDir)
		if err := mgr.RecordSkillWithAgent(hookInput.SessionID, skillName, hookInput.Agent); err != nil {
			logger.Warn("failed to record skill invocation", "error", err)
		}
		return writeAllow(cmd, hookAdapter)
	}

	// Step 5b: Check if this is a Read of a SKILL.md file — auto-record the skill.
	// This allows agents without a native Skill tool (like Cursor) to load skills
	// by reading the skill file.
	if hookInput.ToolName == "Read" && hookInput.FilePath != "" {
		if skillName := detectSkillFromPath(hookInput.FilePath); skillName != "" {
			logger.Debug("skill file read detected", "skill", skillName, "path", hookInput.FilePath)
			logSkillEvent(projectRoot, hookInput, skillName, "read")
			stateDir := filepath.Join(projectRoot, ".care-bear", "state")
			if err := os.MkdirAll(stateDir, 0o755); err == nil {
				mgr := state.NewStateManager(stateDir)
				if err := mgr.RecordSkillWithAgent(hookInput.SessionID, skillName, hookInput.Agent); err != nil {
					logger.Warn("failed to record skill from file read", "error", err)
				}
			}
			// Don't short-circuit — let the Read proceed normally
		}
	}

	// Step 6: Load enforcement config.
	// Priority: repo-keyed config dir > project-level .care-bear/
	var rules []engine.MatchedRule
	if repoConfigDir != "" {
		rules, err = engine.LoadConfigFromDir(repoConfigDir)
		if err != nil {
			logger.Warn("failed to load repo config, trying project config", "error", err)
			rules = nil
		}
	}
	if len(rules) == 0 {
		rules, err = engine.LoadConfig(projectRoot)
	}
	if err != nil {
		// Malformed JSON is surfaced as an error; other config errors fail open.
		logger.Warn("failed to load config, allowing operation", "error", err)
		return writeAllow(cmd, hookAdapter)
	}
	logger.Debug("loaded enforcement rules", "count", len(rules))

	if len(rules) == 0 {
		logger.Debug("no enforcement rules, allowing")
		return writeAllow(cmd, hookAdapter)
	}

	// Step 7: Load session state with skill TTL.
	globalCfg, cfgErr := engine.LoadGlobalConfig(projectRoot)
	if cfgErr != nil {
		logger.Warn("failed to load global config, using defaults", "error", cfgErr)
		globalCfg = &engine.GlobalConfig{StateTTLHours: 24}
	}
	skillTTL := time.Duration(globalCfg.SkillTTLMinutes) * time.Minute

	stateDir := filepath.Join(projectRoot, ".care-bear", "state")
	var invokedSkills map[string]bool
	if _, err := os.Stat(stateDir); err == nil {
		mgr := state.NewStateManager(stateDir)
		invokedSkills, err = mgr.GetFreshSkills(hookInput.SessionID, skillTTL)
		if err != nil {
			logger.Warn("failed to read session state, treating as empty", "error", err)
			invokedSkills = make(map[string]bool)
		}
	} else {
		invokedSkills = make(map[string]bool)
	}
	logger.Debug("loaded invoked skills", "count", len(invokedSkills), "ttl_minutes", globalCfg.SkillTTLMinutes)

	// Step 8: Normalize the file path.
	normalizedPath := engine.NormalizeFilePath(hookInput.FilePath, projectRoot)
	logger.Debug("normalized file path", "original", hookInput.FilePath, "normalized", normalizedPath)

	// Step 9: Evaluate enforcement.
	blockResult := engine.ShouldBlock(rules, hookInput.ToolName, normalizedPath, hookInput.Agent, invokedSkills)
	logger.Debug("enforcement decision", "blocked", blockResult.Blocked, "missing", blockResult.Missing)

	// Step 10: Log only enforcement-relevant events (BLOCK or ALLOW with matched rules).
	// Only log enforcement-relevant events — skip actions with no matching rules.
	matched := engine.MatchedSkills(rules, hookInput.ToolName, normalizedPath, hookInput.Agent)
	logger.Debug("matched skills for logging", "matched", matched, "tool", hookInput.ToolName, "path", normalizedPath)
	if len(matched) > 0 {
		// For BLOCK events, the relevant skills come from blockResult.Missing.
		// For ALLOW events, the relevant skills come from the matched parameter.
		// We pass matchedSkills separately to avoid mutating blockResult.Missing.
		logEvent(projectRoot, hookInput, normalizedPath, blockResult, matched)
	}

	// Step 11: Output response.
	if blockResult.Blocked {
		denyBytes, err := hookAdapter.FormatDeny(blockResult.Reason)
		if err != nil {
			return fmt.Errorf("formatting deny response: %w", err)
		}
		fmt.Fprint(cmd.OutOrStdout(), string(denyBytes))
		logger.Debug("denied tool invocation", "reason", blockResult.Reason)
	} else {
		if err := writeAllow(cmd, hookAdapter); err != nil {
			return err
		}
		logger.Debug("allowed tool invocation")
	}

	// Step 12: Trigger throttled pruning (reuses globalCfg loaded in step 7).
	if _, err := os.Stat(stateDir); err == nil {
		ttl := time.Duration(globalCfg.StateTTLHours) * time.Hour
		if pruneErr := state.PruneIfDue(stateDir, ttl); pruneErr != nil {
			logger.Warn("pruning failed", "error", pruneErr)
		}
	}

	// For Cursor: exit code 2 signals "block this action".
	// Claude Code reads the deny decision from stdout JSON (exit 0 is fine).
	// Return ExitError so main.go calls os.Exit(2) after cleanup completes.
	if blockResult.Blocked && hookInput.Agent == "cursor" {
		return &ExitError{Code: 2}
	}

	return nil
}

// writeAllow writes the allow response to stdout and returns nil.
// For Claude Code, this is empty output (exit 0 with no stdout = allow).
func writeAllow(cmd *cobra.Command, a adapter.HookAdapter) error {
	allowBytes, err := a.FormatAllow()
	if err != nil {
		return fmt.Errorf("formatting allow response: %w", err)
	}
	if len(allowBytes) > 0 {
		fmt.Fprint(cmd.OutOrStdout(), string(allowBytes))
	}
	return nil
}

// detectSkillFromPath checks if a file path points to a skill definition file
// (SKILL.md) and extracts the skill name from the parent directory.
// Examples:
//   - ".claude/skills/run-migration/SKILL.md" → "run-migration"
//   - "/abs/path/.claude/skills/git/SKILL.md" → "git"
//   - "some/other/file.go" → "" (not a skill file)
func detectSkillFromPath(filePath string) string {
	// Normalize to forward slashes
	normalized := strings.ReplaceAll(filePath, "\\", "/")

	// Check if it ends with SKILL.md (case-insensitive base name)
	base := filepath.Base(normalized)
	if !strings.EqualFold(base, "SKILL.md") {
		return ""
	}

	// The skill name is the parent directory of SKILL.md
	// e.g., ".claude/skills/run-migration/SKILL.md" → "run-migration"
	dir := filepath.Dir(normalized)
	skillName := filepath.Base(dir)

	if skillName == "." || skillName == "/" || skillName == "" {
		return ""
	}

	return skillName
}

// logSkillEvent logs a skill load event.
// Format matches logEvent: timestamp | agent | tool | path | action | skill
func logSkillEvent(projectRoot string, input *adapter.HookInput, skillName, method string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	logDir := filepath.Join(home, ".care-bear")
	os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, "events.log")
	projectName := filepath.Base(projectRoot)
	if repo := engine.ResolveRepoIdentity(projectRoot); repo != nil {
		projectName = repo.Slug
	}
	line := fmt.Sprintf("%s | %-12s | %-6s | %-5s | SKILL-LOAD | %-40s | LOAD  | %s\n",
		time.Now().UTC().Format(time.RFC3339),
		projectName,
		input.Agent,
		truncateSessionID(input.SessionID),
		"",
		skillName,
	)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line)
}

// logEvent appends a line to .care-bear/events.log for every hook invocation.
// Log format: timestamp | agent | tool | path | decision | skills
// For BLOCK events, skills come from result.Missing (the skills that were not loaded).
// For ALLOW events, skills come from matchedSkills (the skills that were required and satisfied).
// Events older than 7 days are pruned on each write.
func logEvent(projectRoot string, input *adapter.HookInput, normalizedPath string, result engine.BlockResult, matchedSkills []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	logDir := filepath.Join(home, ".care-bear")
	os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, "events.log")
	projectName := filepath.Base(projectRoot)
	if repo := engine.ResolveRepoIdentity(projectRoot); repo != nil {
		projectName = repo.Slug
	}

	decision := "ALLOW"
	skills := matchedSkills
	if result.Blocked {
		decision = "BLOCK"
		skills = result.Missing
	}
	missing := strings.Join(skills, ",")

	line := fmt.Sprintf("%s | %-12s | %-6s | %-5s | %-10s | %-40s | %-5s | %s\n",
		time.Now().UTC().Format(time.RFC3339),
		projectName,
		input.Agent,
		truncateSessionID(input.SessionID),
		input.ToolName,
		normalizedPath,
		decision,
		missing,
	)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line)
}

// truncateSessionID returns the first 5 characters of a session ID.
func truncateSessionID(id string) string {
	if len(id) > 5 {
		return id[:5]
	}
	return id
}
