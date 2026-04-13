# care-bear — How It Works

This document explains the complete flow: how hooks intercept agent actions, how enforcement decisions are made, how state is tracked per session, and how the TUI provides observability.

## The Big Picture

```
AI Agent (Claude Code / Cursor / future agents)
    |
    | Agent performs a tool call (Edit, Write, Bash, etc.)
    | Agent's hook system sends JSON to care-bear via stdin
    v
care-bear hook (PreToolUse)
    |
    +-- 1. Parse agent-specific JSON via adapter
    +-- 2. Detect skill invocations → record in session state
    +-- 3. Load enforcement rules from config
    +-- 4. Load session state (which skills are loaded)
    +-- 5. Evaluate: ShouldBlock(rules, tool, path, agent, skills)
    |
    +-- ALLOW → agent proceeds normally
    +-- BLOCK → agent shows "Load skill by running: /skill-name"
                Agent loads the skill, retries, succeeds
```

## Enforcement Rules

Rules are defined in `skill_enforcement.json`:

```json
{
  "version": 1,
  "tools": [
    {"tool": "Edit", "path": "**/*.go", "skill": "go-standards", "agent": "*"}
  ]
}
```

Each rule says: "Before using `tool` on files matching `path` for `agent`, the skill `skill` must be loaded in the current session."

### Rule Matching

`ShouldBlock()` in `internal/engine/engine.go` is a **pure function** — no side effects, no I/O:

1. For each rule, check if `tool` matches (exact or `*` wildcard)
2. Check if `path` matches the file being accessed (doublestar glob)
3. Check if `agent` matches (`claude`, `cursor`, or `*`)
4. If all three match, check if `skill` is in the session's loaded skills
5. If any matched rule's skill is NOT loaded → **BLOCK**

### Config Locations (priority order)

1. **Repo-keyed config**: `~/.care-bear/repos/{hash}-{slug}/skill_enforcement.json` — per-repo rules stored outside the project directory
2. **Project-level**: `{project}/.care-bear/skill_enforcement.json` — checked into the repo
3. **User-level**: `~/.care-bear/skill_enforcement.json` — personal defaults

## Session State

State is tracked per-session using JSON files on disk:

```
{project}/.care-bear/state/
  {session-id}.json     # Session state
  {session-id}.lock     # Advisory lock for concurrency safety
```

Each state file contains:

```json
{
  "session_id": "abc123",
  "agent": "claude",
  "created_at": "2024-01-01T00:00:00Z",
  "invoked_skills": ["go-standards", "linear"],
  "skill_timestamps": {
    "go-standards": "2024-01-01T10:30:00Z",
    "linear": "2024-01-01T10:35:00Z"
  }
}
```

### Skill TTL

Skills can expire after a configurable time (set in `config.json`):
- `skill_ttl_minutes: 0` — no expiry (default, backward compatible)
- `skill_ttl_minutes: 60` — skills expire after 1 hour, must be re-loaded

When checking if a skill is loaded, `GetFreshSkills()` compares each skill's timestamp against the TTL. Expired skills are treated as not loaded.

### Concurrency Safety

Multiple agent sessions may run simultaneously. State files use:
- **Advisory file locks** (`gofrs/flock`) — exclusive lock for writes, shared lock for reads
- **Atomic writes** (`natefinch/atomic`) — no partial/corrupt state files on crash

### State Pruning

Old session state files are automatically cleaned up:
- `state_ttl_hours` (default: 24) controls how long state files are kept
- `PruneIfDue()` runs at most once per hour during hook invocations
- Uses file modification time (mtime) — no need to parse JSON for expiry checks

## Agent Adapters

Each AI agent has its own hook format. Adapters normalize these into a common `HookInput`:

### Claude Code

- **Hook type**: PreToolUse (configured in `~/.claude/settings.json`)
- **Input format**: JSON with `session_id`, `tool_name`, `tool_input.file_path`, `cwd`
- **Allow**: Exit 0, empty stdout
- **Deny**: Exit 0, JSON with `hookSpecificOutput.permissionDecision: "deny"`
- **Skill detection**: Native `Skill` tool → `tool_name == "Skill"`, extract `tool_input.skill`

### Cursor

- **Hook type**: preToolUse (configured in `~/.cursor/hooks.json`)
- **Input format**: JSON with `conversation_id`, `tool_name`, `file_path`, `workspace_roots`
- **Allow**: Exit 0, `{"continue": true}`
- **Deny**: Exit 2, `{"continue": false, "userMessage": "..."}`
- **Skill detection**: No native Skill tool. Auto-detect SKILL.md file reads instead.
- **Tool name normalization**: Maps `edit_file` → `Edit`, `write_file` → `Write`, etc.

### Adding a New Agent

1. Create `internal/adapter/myagent.go` implementing `HookAdapter`
2. Register in `internal/adapter/registry.go`
3. That's it — engine, TUI, CLI, state all work automatically

## Project Identity

Projects are identified by their Git repository, not their local directory:

- `git remote get-url origin` → normalize SSH/HTTPS/token URLs → `org/repo` slug
- Same repo checked out in multiple directories is treated as one project
- Config stored at `~/.care-bear/repos/{hash}-{slug}/` — keyed by repo identity
- Users can set a preferred checkout path in `preferences.json`

## Global Event Log

All enforcement decisions are logged to `~/.care-bear/events.log`:

```
2024-01-01T10:30:00Z | Blue-Bear-Security/blueden | claude | abc12 | Edit | services/bff/handler.py | BLOCK | backend-python-standards
```

Columns: timestamp, project (repo slug), agent, session (5 chars), tool, path, action, skill(s)

The TUI dashboard shows this log with:
- Real-time updates via fsnotify
- Multi-column filtering (action, project, session, agent, tool, skill)
- Color coding: red = BLOCK, green = ALLOW, cyan = SKILL-LOAD

## TUI Architecture

The TUI uses Charmbracelet Bubble Tea with a hierarchy of models:

```
App (root model)
  ├── Dashboard (split-pane: skills + rules + event log)
  ├── RuleEditor (three pinned sections: tools, paths, agents)
  ├── TreePicker (file browser for path selection)
  └── Settings (global/project config editor)
```

All scrollable lists use the shared `ScrollView` component (`internal/tui/scroll.go`) — one implementation for cursor tracking, viewport follow, page up/down, and jump to top/bottom.
