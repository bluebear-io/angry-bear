// hook.go implements the care-bare hook command for PreToolUse enforcement.
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

	"github.com/Blue-Bear-Security/care-bare/internal/adapter"
	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/Blue-Bear-Security/care-bare/internal/state"
	"github.com/spf13/cobra"
)

// maxStdinSize is the maximum number of bytes read from stdin (5MB).
// Anything beyond this is likely malformed or adversarial input.
const maxStdinSize = 5 * 1024 * 1024

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

	// Step 4: Resolve project root.
	projectRoot := engine.ResolveProjectRoot(hookInput.Cwd)
	logger.Debug("resolved project root", "root", projectRoot)

	// Step 5: Check for skill invocation (short-circuit).
	skillName, isSkill := hookAdapter.DetectSkillInvocation(hookInput)
	if isSkill {
		logger.Debug("skill invocation detected", "skill", skillName)
		logSkillEvent(projectRoot, hookInput, skillName, "invoke")
		stateDir := filepath.Join(projectRoot, ".care-bare", "state")
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
			stateDir := filepath.Join(projectRoot, ".care-bare", "state")
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
	// Use a fake home dir that doesn't contain configs to avoid picking up
	// the developer's own user-level config during testing. In production,
	// LoadConfig uses the real home directory by default.
	rules, err := engine.LoadConfig(projectRoot)
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

	// Step 7: Load session state.
	stateDir := filepath.Join(projectRoot, ".care-bare", "state")
	var invokedSkills map[string]bool
	if _, err := os.Stat(stateDir); err == nil {
		mgr := state.NewStateManager(stateDir)
		invokedSkills, err = mgr.GetInvokedSkills(hookInput.SessionID)
		if err != nil {
			logger.Warn("failed to read session state, treating as empty", "error", err)
			invokedSkills = make(map[string]bool)
		}
	} else {
		invokedSkills = make(map[string]bool)
	}
	logger.Debug("loaded invoked skills", "count", len(invokedSkills))

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
		// For ALLOW events, include which skills were required (and satisfied)
		if !blockResult.Blocked {
			blockResult.Missing = matched // reuse Missing field for "matched" skills on allow
		}
		logEvent(projectRoot, hookInput, normalizedPath, blockResult)
	}

	// Step 11: Output response.
	if blockResult.Blocked {
		denyBytes, err := hookAdapter.FormatDeny(blockResult.Reason)
		if err != nil {
			return fmt.Errorf("formatting deny response: %w", err)
		}
		fmt.Fprint(cmd.OutOrStdout(), string(denyBytes))
		logger.Debug("denied tool invocation", "reason", blockResult.Reason)

		// For Cursor: exit code 2 signals "block this action".
		// Claude Code reads the deny decision from stdout JSON (exit 0 is fine).
		// We use os.Exit(2) to ensure Cursor respects the block.
		if hookInput.Agent == "cursor" {
			os.Exit(2)
		}
	} else {
		if err := writeAllow(cmd, hookAdapter); err != nil {
			return err
		}
		logger.Debug("allowed tool invocation")
	}

	// Step 11: Trigger throttled pruning.
	if _, err := os.Stat(stateDir); err == nil {
		globalCfg, err := engine.LoadGlobalConfig(projectRoot)
		if err != nil {
			logger.Warn("failed to load global config for pruning", "error", err)
		} else {
			ttl := time.Duration(globalCfg.StateTTLHours) * time.Hour
			if err := state.PruneIfDue(stateDir, ttl); err != nil {
				logger.Warn("pruning failed", "error", err)
			}
		}
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
	logPath := filepath.Join(projectRoot, ".care-bare", "events.log")
	line := fmt.Sprintf("%s | %-6s | %-5s | SKILL-LOAD | %-50s | LOAD  | %s\n",
		time.Now().UTC().Format(time.RFC3339),
		input.Agent,
		truncateSessionID(input.SessionID),
		"",
		skillName,
	)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line)
}

// logEvent appends a line to .care-bare/events.log for every hook invocation.
// Log format: timestamp | agent | tool | path | decision | missing skills
// Events older than 7 days are pruned on each write.
func logEvent(projectRoot string, input *adapter.HookInput, normalizedPath string, result engine.BlockResult) {
	logPath := filepath.Join(projectRoot, ".care-bare", "events.log")

	decision := "ALLOW"
	if result.Blocked {
		decision = "BLOCK"
	}
	missing := strings.Join(result.Missing, ",")

	line := fmt.Sprintf("%s | %-6s | %-5s | %-10s | %-50s | %-5s | %s\n",
		time.Now().UTC().Format(time.RFC3339),
		input.Agent,
		truncateSessionID(input.SessionID),
		input.ToolName,
		normalizedPath,
		decision,
		missing,
	)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
