# Architecture

This document describes the internal architecture of care-bear for contributors and advanced users.

## Overview

care-bear is a single Go binary with no runtime dependencies. It operates in two modes:

1. **Interactive TUI** — a machine-level terminal dashboard (launched with `care-bear`) that discovers all projects across all AI agents, lets you pick one, and configure enforcement rules.
2. **Headless hook** — a pre-tool-use hook (launched with `care-bear hook`) that reads JSON from stdin, evaluates enforcement rules, and writes allow/deny JSON to stdout.

The CLI is an **observability and configuration tool**. All enforcement work is done by the hooks.

### Trust Model

care-bear is a developer productivity tool, not a security perimeter. It assumes a trusted local environment where the goal is team discipline — ensuring agents load the right context before modifying files.

## Project Structure

```
care-bear/
  cmd/
    care-bear/
      main.go              # Entry point: sets version info + binary path, calls cli.Execute()
  internal/
    adapter/               # Agent-specific logic (ALL agent knowledge lives here)
      adapter.go           #   HookAdapter interface + AgentProject types
      types.go             #   HookInput struct (normalized agent-agnostic input)
      claude.go            #   Claude Code adapter + project scanner
      cursor.go            #   Cursor adapter + tool name normalization + project scanner
      registry.go          #   Adapter registry, auto-detection, ScanAllProjects()
    cli/                   # Cobra commands
      root.go              #   Root command (project picker → TUI)
      hook.go              #   hook subcommand (enforcement + event logging)
      init_cmd.go          #   init subcommand (project bootstrapping)
      status.go            #   status subcommand
      clean.go             #   clean subcommand
      doctor.go            #   doctor subcommand
      version.go           #   version subcommand
    engine/                # Core enforcement logic (agent-agnostic)
      types.go             #   Rule, Config, GlobalConfig, MatchedRule, BlockResult
      config.go            #   Two-level config loading and merging
      engine.go            #   ShouldBlock algorithm (pure function)
      glob.go              #   Glob/path normalization, project root resolution
    scanner/               # Skill discovery
      types.go             #   Skill struct
      scanner.go           #   Directory walking, skill file detection
      parser.go            #   YAML frontmatter parsing
      projects.go          #   (legacy, being replaced by adapter.ScanProjects)
    state/                 # Session state management
      types.go             #   SessionState + LoadedSkill types
      manager.go           #   StateManager: RecordSkillWithAgent, HasSkill, etc.
      lock.go              #   FileLock wrapper (gofrs/flock)
      prune.go             #   TTL-based pruning with throttle
      validate.go          #   Session ID sanitization
    tui/                   # Interactive terminal UI
      app.go               #   Root Bubble Tea model, fsnotify watchers, view routing
      dashboard.go         #   Split-pane: skills list (left) + rules/description (right)
      rule_editor.go       #   Multi-select rule builder (tools, paths tree, agents)
      tree_picker.go       #   File tree browser with smart filtering
      styles.go            #   Lip Gloss style definitions
```

## Adapter Architecture (Extensibility)

**All agent-specific logic lives in adapters.** To add support for a new agent (e.g., Codex, Gemini), implement the `HookAdapter` interface:

```go
type HookAdapter interface {
    Name() string                                              // "claude", "cursor", "codex"
    ParseInput(stdin io.Reader) (*HookInput, error)            // Normalize agent JSON → HookInput
    FormatAllow() ([]byte, error)                              // Agent-specific allow response
    FormatDeny(reason string) ([]byte, error)                  // Agent-specific deny response
    ConfigPath() string                                        // Where to detect agent presence
    InstallHook(projectDir string) error                       // Install hooks in agent's global config
    DetectSkillInvocation(input *HookInput) (string, bool)     // Detect native skill loading
    ScanProjects() ([]AgentProject, error)                     // Discover projects for this agent
}
```

Register it in `registry.go`. Everything else — engine, TUI, commands, state — works automatically.

### What Each Adapter Handles

| Concern | Claude Code | Cursor |
|---------|------------|--------|
| Hook JSON format | `session_id`, nested `tool_input.file_path` | `conversation_id`, top-level or nested `file_path` |
| Tool name normalization | Native names (Edit, Write, etc.) | Maps `edit_file`→Edit, `write_file`→Write, etc. |
| Allow response | Empty stdout, exit 0 | `{"continue":true}`, exit 0 |
| Deny response | `hookSpecificOutput` JSON, exit 0 | `{"continue":false,"permission":"deny"}`, exit 2 |
| Skill detection | Native Skill tool (`tool_name: "Skill"`) | Auto-detect SKILL.md reads |
| Hook config location | `~/.claude/settings.json` | `~/.cursor/hooks.json` |
| Project discovery | `~/.claude/projects/` (sessions-index.json + greedy decode) | `~/.cursor/projects/` (greedy path decode) |

### Adapter Registry

The registry provides:
- `Get(name)` — lookup by name
- `AutoDetect(rawJSON)` — detect agent from JSON markers (`cursor_version` → Cursor, `hook_event_name` → Claude)
- `ScanAllProjects()` — discovers projects from ALL adapters, merges duplicates (same project used by multiple agents)

## Enforcement Engine (`internal/engine/`)

The engine is agent-agnostic. It only knows about rules, paths, and skills.

### ShouldBlock Algorithm

Pure function with no side effects:

1. For each rule, check: **tool match** AND **agent match** AND **path match**
2. Deduplicate by skill name (first match per skill wins)
3. Check which matched skills are NOT in the session's invoked skills
4. Return `BlockResult{Blocked, Reason, Missing}`

The deny message includes load instructions: `/skill-name (or read .claude/skills/skill-name/SKILL.md)`

### Config Loading

Two levels, both accumulate (no override):
1. User-level: `~/.care-bear/skill_enforcement.json`
2. Project-level: walk up from cwd, collect all `.care-bear/skill_enforcement.json`

### Path Handling

- Relative globs auto-prefix with `**/` 
- File paths from agents normalized to repo-relative, forward-slash format
- Project root: walk up looking for `.care-bear/`, fall back to `.git/`, fall back to cwd

## State Manager (`internal/state/`)

Tracks which skills are loaded per session per agent.

### State File Format

```json
{
  "session_id": "abc123",
  "agent": "cursor",
  "created_at": "2026-04-12T09:30:00Z",
  "invoked_skills": ["git", "run-migration"]
}
```

### Safety
- File locks (`gofrs/flock`) for concurrent access
- Atomic writes (`natefinch/atomic`) to prevent corruption
- Session ID sanitization: `[A-Za-z0-9._-]`, max 128 chars

### Pruning
- TTL-based (default 24h) using file mtime
- Throttled to max once per hour during hook calls
- `care-bear clean` bypasses throttle

## Event Logging

Every hook invocation is logged to `.care-bear/events.log`:

```
2026-04-12T11:57:01Z | cursor | Write      | console/src/app/layout.tsx  | BLOCK | git
2026-04-12T11:57:04Z | cursor | SKILL-LOAD | git                         | read  | e0a31eac
2026-04-12T11:57:07Z | cursor | Write      | console/src/app/layout.tsx  | ALLOW |
```

## Hook Data Flow

```
AI Agent fires PreToolUse
    |
    v
care-bear hook --agent <name>
    |
    +-- 1. Read stdin (5MB limit)
    +-- 2. Select adapter (--agent flag or auto-detect)
    +-- 3. Parse + normalize input (tool names, file paths)
    +-- 4. Resolve project root
    +-- 5a. Skill tool invocation? → Record skill + allow
    +-- 5b. SKILL.md file read? → Auto-record skill (Cursor support)
    +-- 6. Load enforcement rules
    +-- 7. Load session state (invoked skills)
    +-- 8. Normalize file path
    +-- 9. ShouldBlock()
    +-- 10. Log event to events.log
    +-- 11. Output allow/deny response
    +-- 12. Throttled state pruning
```

## TUI Architecture

### Project Picker
On startup, the TUI calls `registry.ScanAllProjects()` to discover all projects across all agents. Shows a `huh` Select form with project paths and which agents use them.

### Split-Pane Dashboard
After selecting a project:
- **Left panel**: Scrollable skills list with rule counts and loaded-skill badges (`claude`, `cursor`)
- **Right panel**: Selected skill's full description, rules table, inline edit actions

### Rule Editor
Multi-select rule builder with one continuous scrollable list:
- TOOLS section (checkboxes)
- PATHS section (tree walker with expand/collapse)
- AGENTS section (checkboxes)
- `ctrl+s` saves, `esc` cancels

### Real-time Updates
- **fsnotify** watches `.care-bear/state/` for skill load changes
- Badges update live when agents load skills in other sessions

## Hooks Installation

Hooks are installed **globally** for each agent:
- Claude Code: `~/.claude/settings.json` → `hooks.PreToolUse`
- Cursor: `~/.cursor/hooks.json` → `preToolUse`, `beforeShellExecution`, `beforeReadFile`, `beforeMCPExecution`

Uses absolute path to the binary so it works regardless of PATH. Idempotent — safe to run multiple times.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Config missing | No rules = allow all |
| Config malformed JSON | Error (surface to user) |
| State dir missing | No skills invoked |
| State read/write error | Log warning, fail-open |
| Lock acquisition failure | Log warning, fail-open |
| Glob pattern error | Skip the rule |

The hook exits 0 for both allow and deny (Claude Code). For Cursor, deny exits with code 2. Non-zero exit codes indicate actual errors (malformed input, fatal I/O).
