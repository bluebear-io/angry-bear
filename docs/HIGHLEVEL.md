# care-bear — How It Works

This document explains the complete internals: enforcement flow, file management, state lifecycle, real-time updates, and hook installation.

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
    +-- 4. Load session state (which skills are loaded, check TTL)
    +-- 5. Evaluate: ShouldBlock(rules, tool, path, agent, skills)
    |
    +-- ALLOW → agent proceeds normally
    +-- BLOCK → "Load skill by running: /skill-name"
                Agent loads the skill, retries, succeeds
```

## Data Layout

ALL care-bear data lives under `~/.care-bear/`. Nothing is stored in project directories.

```
~/.care-bear/
│
├── config.json                                    # Global config defaults
├── events.log                                     # Global enforcement event log (append-only)
│
└── repos/
    ├── 5ce4353d-Blue-Bear-Security-blueden/        # Per-repo directory
    │   ├── skill_enforcement.json                  #   Enforcement rules
    │   ├── config.json                             #   Per-repo config overrides
    │   ├── preferences.json                        #   Preferred checkout path
    │   └── state/                                  #   Session state
    │       ├── {session-id}.json                   #     Loaded skills + timestamps
    │       ├── {session-id}.lock                   #     Advisory lock file
    │       └── .last-prune                         #     Timestamp of last prune run
    │
    └── bb1bf16d-Blue-Bear-Security-care-bear/      # Another repo
        └── ...
```

Directory name format: `{hash}-{slug}` where hash = first 8 chars of SHA-256(`org/repo`).

### File Descriptions

| File | Created when | Updated when | Read by |
|------|-------------|--------------|---------|
| `~/.care-bear/config.json` | User runs TUI settings (global level) | Settings page saves | Hook (for skill_ttl, state_ttl), all commands |
| `~/.care-bear/events.log` | First enforcement event | Every BLOCK, ALLOW, or SKILL-LOAD | TUI dashboard (read + fsnotify watch) |
| `repos/{hash}/skill_enforcement.json` | First `care-bear add` for this repo | `add`, `rm`, TUI rule editor | Hook (every invocation), `rules`, `status`, `doctor` |
| `repos/{hash}/config.json` | TUI settings (project level) | Settings page saves | Hook, TUI (overrides global defaults) |
| `repos/{hash}/preferences.json` | User picks a checkout path | TUI settings, project picker | TUI project picker, settings |
| `repos/{hash}/state/{session}.json` | First skill load or enforcement in a session | Every skill invocation (timestamp refresh) | Hook (every invocation to check loaded skills) |
| `repos/{hash}/state/{session}.lock` | Alongside state file | Held during read/write | State manager (concurrency safety) |
| `repos/{hash}/state/.last-prune` | First prune run | After each prune (at most hourly) | Prune throttle check |

## Enforcement Rules

Rules in `skill_enforcement.json`:

```json
{
  "version": 1,
  "tools": [
    {"tool": "Edit", "path": "**/*.go", "skill": "go-standards", "agent": "*"}
  ]
}
```

Each rule: "Before using `tool` on files matching `path` for `agent`, skill `skill` must be loaded."

### Rule Matching (pure function, no I/O)

`ShouldBlock()` in `internal/engine/engine.go`:

1. For each rule, check `tool` matches (exact or `*` wildcard)
2. Check `path` matches the file (doublestar glob)
3. Check `agent` matches (`claude`, `cursor`, or `*`)
4. If all match, check `skill` in session's loaded skills
5. Any matched rule's skill NOT loaded → **BLOCK**

## Session State Lifecycle

### Skill Loading

When an agent loads a skill:
1. **Claude Code**: Agent runs `/skill-name` → hook sees `tool_name == "Skill"` → records skill + timestamp
2. **Cursor**: Agent reads `.claude/skills/skill-name/SKILL.md` → hook detects the path pattern → records skill + timestamp

Recording: `RecordSkillWithAgent(sessionID, skillName, agent)` in `internal/state/manager.go`
- Creates `{session}.json` if it doesn't exist
- Appends skill to `invoked_skills` list (idempotent)
- Updates `skill_timestamps[skillName]` to current time (refreshes on re-load)
- Uses exclusive file lock + atomic write

### Skill TTL

`config.json` → `skill_ttl_minutes` (default: 0 = no expiry)

When checking loaded skills, `GetFreshSkills(sessionID, ttl)`:
- If `ttl == 0`: return all skills (no expiry)
- Otherwise: compare each skill's timestamp against TTL, exclude expired ones
- Skills without timestamps (old state files) treated as fresh (backward compat)

**Expiry lifecycle**: LOAD → ALLOW (skill fresh) → EXPIR (TTL exceeded) → BLOCK (skill must be reloaded)

**EXPIR deduplication**: When a skill expires, the hook logs a single EXPIR event and marks it in
`expired_skills` on the session state. Subsequent hook calls skip re-logging the same expiry.
Reloading the skill clears the expired flag via `RecordSkill()`.

Session state fields for TTL:
- `skill_timestamps` — maps skill name → RFC3339 load time (refreshed on reload)
- `expired_skills` — maps skill name → `true` when expiry has been logged (prevents duplicate EXPIR events)

### Concurrency Safety

Multiple agent sessions may run simultaneously:
- **Advisory file locks** (`gofrs/flock`) — exclusive for writes, shared for reads
- **Atomic writes** (`natefinch/atomic`) — no partial/corrupt files on crash
- Lock files: `{session}.lock` alongside `{session}.json`

### State Pruning

Runs automatically during hook invocations (throttled to once per hour):
- `state_ttl_hours` (default: 24) controls file retention
- Uses file modification time (mtime) — no JSON parsing needed
- `.last-prune` file tracks when pruning last ran
- Explicit: `care-bear clean [--all] [--session <id>]`

## Event Log

`~/.care-bear/events.log` — append-only, one line per event:

```
2024-01-01T10:30:00Z | blueden | claude | abc12 | Edit       | services/bff/handler.py | BLOCK | backend-python-standards
2024-01-01T10:30:00Z | blueden | claude | abc12 | SKILL-LOAD |                         | LOAD  | linear
2024-01-01T10:31:05Z | blueden | claude | abc12 | SKILL-TTL  |                         | EXPIR | linear
```

Columns: `timestamp | project | agent | session(5ch) | tool | path | action | skills`

Actions: `BLOCK`, `ALLOW`, `LOAD` (skill loaded), `EXPIR` (skill TTL expired)

### Written by
- Hook: on every BLOCK or ALLOW that matched rules, every SKILL-LOAD, and once per expired skill per session (EXPIR)
- Format: `logEvent()` and `logSkillEvent()` in `internal/cli/hook.go`

### Read by
- TUI dashboard: `LoadEventLog()` reads the file, filters to last 7 days
- Only events with skill associations are shown (noise-free)

### Real-time Updates (fsnotify)

The TUI watches two paths for live updates:

1. **`~/.care-bear/events.log`** — `watchEventsLog()` in `app.go`
   - On file write/create → sends `eventsUpdatedMsg` → dashboard reloads log + auto-scrolls to latest
   - Watcher restarts after each event (single-shot pattern)

2. **`repos/{hash}/state/`** — `watchStateDir()` in `app.go`
   - On `.json` file write/create → sends `loadedSkillsUpdatedMsg` → dashboard updates skill badges
   - Shows which skills are loaded per agent (claude/cursor badges)

Both watchers use `fsnotify` and restart after each event to avoid goroutine leaks.

## Hook Installation

### Auto-install (PersistentPreRunE)

Every CLI command (except `hook`, `completion`, `version`, `enable`, `disable`) runs `EnsureHooksInstalled()` before executing:

1. For each registered adapter (claude, cursor):
   - Check if `~/.{agent}/` exists (agent present on machine)
   - Check if hook config already contains "care-bear hook" (already installed)
   - If not installed: call `adapter.InstallHook("")`
2. Print `✓ Hooks installed for claude` on first install (silent if already installed)

### Claude Code hooks

Written to `~/.claude/settings.json` → `hooks.PreToolUse[]`:
```json
{
  "matcher": "*",
  "hooks": [{"type": "command", "command": "/path/to/care-bear hook --agent claude"}]
}
```
Uses absolute binary path from `os.Executable()` for reliability.

### Cursor hooks

Written to `~/.cursor/hooks.json` → `hooks.{hookType}[]`:
- Prepended to: `preToolUse`, `beforeFileEdit`, `beforeShellExecution`, `beforeReadFile`, `beforeMCPExecution`
- Format: `{"command": "/path/to/care-bear hook --agent cursor"}`
- Cursor blocks on exit code 2 (unlike Claude which reads stdout JSON)

### enable / disable commands

- `care-bear enable` — calls `InstallHook()` on all detected agents
- `care-bear disable` — calls `UninstallHook()` on all detected agents
  - Claude: filters care-bear entries from `settings.json` `PreToolUse[]`
  - Cursor: filters care-bear entries from all hook type arrays in `hooks.json`
  - Preserves all non-care-bear hooks

## Project Discovery

### TUI (rescans every startup)

1. **Claude Code**: Scans `~/.claude/projects/` — encoded directory names
2. **Cursor**: Scans `~/.cursor/projects/` — same encoding

For each directory:
1. Try `sessions-index.json` for real project path (most reliable)
2. Fall back to greedy path decoding from directory name
3. Resolve Git identity → `org/repo` slug
4. Merge: same repo from multiple agents → one project entry

### CLI commands

`add`, `rules`, `status`, `doctor` use current directory:
1. Walk up from `os.Getwd()` looking for `.git/` → project root
2. `git remote get-url origin` → normalize → repo identity
3. Config at `~/.care-bear/repos/{hash}-{slug}/`

## Agent Adapters

Each agent has its own hook format. The `HookAdapter` interface normalizes everything:

### Claude Code
- **Hook config**: `~/.claude/settings.json`
- **Allow**: Exit 0, empty stdout
- **Deny**: Exit 0, JSON with `hookSpecificOutput.permissionDecision: "deny"`
- **Skill detection**: `tool_name == "Skill"` → extract `tool_input.skill`

### Cursor
- **Hook config**: `~/.cursor/hooks.json`
- **Allow**: Exit 0, `{"continue": true}`
- **Deny**: Exit 2, `{"continue": false, "userMessage": "..."}`
- **Skill detection**: Auto-detect SKILL.md file reads (no native Skill tool)
- **Tool normalization**: `edit_file` → `Edit`, `write_file` → `Write`, etc.

### Adding a New Agent

1. Implement `HookAdapter` interface in `internal/adapter/myagent.go`
2. Register in `internal/adapter/registry.go`
3. Everything else works automatically

## TUI Architecture

Charmbracelet Bubble Tea with shared components:

```
App (root model)
  ├── Dashboard (split-pane: skills + rules + event log)
  ├── RuleEditor (three pinned sections: tools, paths, agents)
  ├── TreePicker (file browser for path selection)
  └── Settings (global/project config editor)
```

- **ScrollView** (`scroll.go`) — shared by all scrollable lists
- **ParsedEvent** (`event_parser.go`) — shared event log parser
- **Constants** (`constants.go`) — shared tool/agent/ignore lists
