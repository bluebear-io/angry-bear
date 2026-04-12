# Architecture

This document describes the internal architecture of care-bare for contributors and advanced users who want to understand how the tool works.

## Overview

care-bare is a single Go binary with no runtime dependencies. It operates in two modes:

1. **Interactive TUI** -- A terminal dashboard (launched with `care-bare` and no subcommand) for visually configuring enforcement rules.
2. **Headless hook** -- A pre-tool-use hook (launched with `care-bare hook`) that reads JSON from stdin, evaluates enforcement rules, and writes allow/deny JSON to stdout.

### Trust Model

care-bare is a developer productivity tool, not a security perimeter. It assumes a trusted local environment where the goal is team discipline -- ensuring that agents load the right context before modifying files. An agent with file modification access can also modify care-bare's configuration; care-bare does not attempt to prevent this.

## Project Structure

```
care-bare/
  cmd/
    care-bare/
      main.go              # Entry point: sets version info, calls cli.Execute()
  internal/
    adapter/               # Agent-specific hook adapters
      adapter.go           #   HookAdapter interface definition
      types.go             #   HookInput struct (normalized adapter-agnostic input)
      claude.go            #   Claude Code adapter (parse, format, detect, install)
      cursor.go            #   Cursor adapter (parse, format, detect, install)
      registry.go          #   Adapter registry with auto-detection
    cli/                   # Cobra command definitions
      root.go              #   Root command (launches TUI when no subcommand)
      hook.go              #   hook subcommand (pre-tool-use enforcement)
      init_cmd.go          #   init subcommand (project bootstrapping)
      status.go            #   status subcommand (display rules, sessions, skills)
      clean.go             #   clean subcommand (state file cleanup)
      doctor.go            #   doctor subcommand (installation diagnostics)
      version.go           #   version subcommand (print build info)
    engine/                # Core enforcement logic
      types.go             #   Rule, Config, GlobalConfig, MatchedRule, BlockResult
      config.go            #   Two-level config loading and merging
      engine.go            #   ShouldBlock algorithm (pure function)
      glob.go              #   Glob normalization, path normalization, project root resolution
    scanner/               # Skill discovery
      types.go             #   Skill struct and frontmatter types
      scanner.go           #   Directory walking, file detection, deduplication
      parser.go            #   YAML frontmatter parsing for SKILL.md and .mdc files
    state/                 # Session state management
      types.go             #   SessionState struct
      manager.go           #   StateManager: RecordSkill, HasSkill, GetInvokedSkills, Clean
      lock.go              #   FileLock wrapper around gofrs/flock
      prune.go             #   TTL-based pruning with throttle mechanism
      validate.go          #   Session ID sanitization and validation
    tui/                   # Interactive terminal UI
      app.go               #   Root Bubble Tea model, view state machine
      dashboard.go         #   Skills + rules dashboard view
      rule_editor.go       #   Huh form for adding/editing rules
      tree_picker.go       #   File browser for path selection
      styles.go            #   Lip Gloss style definitions
  demo/                    # VHS tape scripts for generating demo GIFs
  docs/                    # Documentation
  test/                    # Integration and E2E tests
```

## Core Components

### Enforcement Engine (`internal/engine/`)

The engine is the heart of care-bare. It is responsible for loading configuration, normalizing patterns, and making the block/allow decision.

**`ShouldBlock` Algorithm:**

The `ShouldBlock` function is a pure function with no side effects. For each rule in the loaded config:

1. Check **tool match**: empty or `*` matches all tools; otherwise exact string match.
2. Check **agent match**: empty or `*` matches all agents; otherwise exact string match.
3. Check **path match**: empty or `*` matches all files; otherwise glob match using `doublestar`.

All three conditions must be true (AND logic) for a rule to match. Matched rules are deduplicated by skill name (first match per skill wins). The function then checks which matched skills have NOT been invoked in the current session.

If any required skills are missing, the function returns `BlockResult{Blocked: true}` with the list of missing skill names and a human-readable reason string.

**Two-Level Config Loading:**

1. User-level rules from `~/.care-bare/skill_enforcement.json`.
2. Project-level rules collected by walking up from the current directory, loading `.care-bare/skill_enforcement.json` at each level.

All rules accumulate. There is no override or shadowing. Each rule tracks its source file path.

**Glob Normalization:**

Relative patterns (not starting with `/` or `**/`) are automatically prefixed with `**/` so they match at any depth. File paths from agent input are normalized to repo-relative, forward-slash format before matching.

**Project Root Resolution:**

care-bare resolves the project root by walking up from the current directory:

1. First directory containing `.care-bare/` (highest priority).
2. First directory containing `.git/` (fallback).
3. The starting directory itself (final fallback).

### State Manager (`internal/state/`)

The state manager tracks which skills have been invoked in each session using JSON files on disk.

**Storage:**

Each session gets a JSON file at `.care-bare/state/{session_id}.json` containing the session ID, creation timestamp, and list of invoked skill names.

**Cross-Process Safety:**

- Advisory file locks via `gofrs/flock` prevent concurrent writes to the same session file.
- Atomic writes via `natefinch/atomic` prevent partial/corrupt state files on crash.
- Read operations use shared (read) locks; write operations use exclusive locks.

**TTL-Based Pruning:**

State files expire after a configurable number of hours (default 24). Expiry is checked by file modification time (mtime) to avoid parsing JSON. A `.last-prune` sentinel file throttles automatic pruning to at most once per hour during hook invocations. The `care-bare clean` command bypasses the throttle.

**Session ID Validation:**

Session IDs are sanitized to prevent path traversal: only `[A-Za-z0-9._-]` characters are allowed, maximum 128 characters, no `..` sequences.

### Hook Adapters (`internal/adapter/`)

Adapters translate between agent-specific JSON formats and care-bare's internal representation.

**`HookAdapter` Interface:**

```go
type HookAdapter interface {
    Name() string
    ParseInput(stdin io.Reader) (*HookInput, error)
    FormatAllow() ([]byte, error)
    FormatDeny(reason string) ([]byte, error)
    ConfigPath() string
    InstallHook(projectDir string) error
    DetectSkillInvocation(input *HookInput) (skillName string, isSkill bool)
}
```

**Adapter Registry:**

The registry maps adapter names to implementations and supports auto-detection from raw JSON:

1. If JSON contains `cursor_version` -> Cursor adapter.
2. If JSON contains `hook_event_name` -> Claude Code adapter.
3. Otherwise -> error.

**Claude Code Adapter:**

- Reads `session_id`, `tool_name`, `tool_input.file_path` from nested JSON.
- Detects skill invocations when `tool_name` is `"Skill"` and extracts the skill name from `tool_input.skill`.
- Allow response: empty stdout (exit 0 with no output).
- Deny response: `{"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": "..."}}`.
- Installs hooks into `.claude/settings.json` under `hooks.PreToolUse`.

**Cursor Adapter:**

- Reads `conversation_id` (as session ID), top-level `tool_name` and `file_path`, and `workspace_roots[0]` (as cwd).
- Allow response: `{"continue": true}`.
- Deny response: `{"continue": false, "userMessage": "..."}`.
- Installs hooks into `.cursor/hooks.json` under multiple hook types: `preToolUse`, `beforeFileEdit`, `beforeShellExecution`, `beforeReadFile`, `beforeMCPExecution`.

### Skill Scanner (`internal/scanner/`)

The scanner discovers skill definitions from configured directory paths.

**File Detection:**

- `SKILL.md` files are identified as Claude Code skills.
- `.mdc` files are identified as Cursor rules.

**Metadata Extraction:**

The parser reads YAML frontmatter (delimited by `---`) from skill files to extract `name` and `description` fields. If no frontmatter is present or the name field is empty, the scanner falls back to the parent directory name.

**Deduplication:**

When the same skill name is discovered in multiple locations, the first occurrence wins. Results are returned sorted alphabetically by name.

### Interactive TUI (`internal/tui/`)

The TUI is built on the Charmbracelet v2 stack: Bubble Tea for the application framework, Lip Gloss for styling.

**View State Machine:**

The TUI uses three views managed by a central App model:

```
Dashboard <-> Rule Editor <-> Tree Picker
```

- **Dashboard** -- Displays all discovered skills with their associated enforcement rules in a scrollable vertical list. Supports navigation (up/down/j/k), editing rules (enter), deleting rules (d), and saving (s).
- **Rule Editor** -- A form for adding or editing a rule. Fields: tool name, path pattern, agent. The skill name is pre-filled from the dashboard context. Supports opening the tree picker (ctrl+t) for path selection.
- **Tree Picker** -- A file browser for selecting path patterns. Filters out directories matching the configured ignore patterns.

Config changes accumulate in memory. The user must explicitly save with the `s` key to write changes to disk.

## Hook Data Flow

Step-by-step walkthrough of a `care-bare hook` invocation:

```
Agent (Claude Code / Cursor)
    |
    | PreToolUse event (JSON via stdin)
    v
care-bare hook
    |
    +-- 1. Read stdin (size-limited to 5MB)
    +-- 2. Select adapter (--agent flag or auto-detect from JSON)
    +-- 3. Parse input via adapter.ParseInput()
    +-- 4. Resolve project root (walk up from cwd)
    +-- 5. Check for skill invocation via adapter.DetectSkillInvocation()
    |       |
    |       +-- If skill: RecordSkill() -> FormatAllow() -> exit
    |
    +-- 6. Load enforcement rules via engine.LoadConfig()
    +-- 7. Load session state via state.GetInvokedSkills()
    +-- 8. Normalize file path to project-relative form
    +-- 9. Evaluate rules via engine.ShouldBlock()
    |       |
    |       +-- Allowed: FormatAllow() -> stdout (empty or JSON)
    |       +-- Blocked: FormatDeny() -> stdout (deny JSON)
    |
    +-- 10. Trigger throttled state pruning
```

### Sequence Diagram

```
Agent -> Hook: stdin JSON
Hook -> Adapter: ParseInput()
Hook -> Adapter: DetectSkillInvocation()
alt Skill invocation
    Hook -> StateManager: RecordSkill()
    Hook -> Adapter: FormatAllow()
    Hook -> Agent: stdout (allow)
else Normal tool use
    Hook -> Engine: LoadConfig()
    Hook -> StateManager: GetInvokedSkills()
    Hook -> Engine: ShouldBlock()
    alt Blocked
        Hook -> Adapter: FormatDeny()
        Hook -> Agent: stdout (deny JSON)
    else Allowed
        Hook -> Adapter: FormatAllow()
        Hook -> Agent: stdout (allow)
    end
end
Hook -> StateManager: PruneIfDue() (throttled)
```

## Config Merge Strategy

Rules from all config sources accumulate (user-level + all project-level configs found by walking up from the current directory). Each rule tracks its source file. There is no override or shadowing -- all rules apply simultaneously. If two rules from different config files match the same tool+path+agent with different skills, both skills must be loaded.

This design favors explicitness: you can always see all active rules and their sources via `care-bare status`.

## Error Handling Philosophy

care-bare follows a **fail-open** strategy for most errors and a **fail-hard** strategy for user mistakes:

| Scenario | Behavior |
|----------|----------|
| Config file missing | Silently skip (no rules enforced) |
| Config file permission denied | Log warning, skip |
| Config file malformed JSON | Return error (surface to user) |
| Config version unsupported | Return error (surface to user) |
| State directory missing | Treat as no skills invoked |
| State file read error | Log warning, treat as no skills invoked |
| State file write error | Log warning, allow the operation |
| Glob pattern error | Skip the rule |

The hook command never exits with a non-zero code for a deny. Deny decisions are communicated via stdout JSON. A non-zero exit code indicates an actual error (malformed input, fatal I/O failure).

## Trust Model

care-bare assumes a trusted local environment. It is a productivity tool for team discipline, not a security boundary.

Key implications:

- An agent that can modify files can also modify care-bare's configuration.
- State files use advisory (not mandatory) locks.
- Session IDs come from the agent and are trusted after sanitization.
- The enforcement is cooperative: agents opt in by having hooks installed.

The goal is to prevent accidental context-free modifications, not to enforce a security policy against a malicious actor.
