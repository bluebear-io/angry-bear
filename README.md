# care-bare

[![CI](https://github.com/Blue-Bear-Security/care-bare/actions/workflows/ci.yml/badge.svg)](https://github.com/Blue-Bear-Security/care-bare/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

**Enforce skill-loading requirements for AI coding agents.**

care-bare prevents AI coding agents (Claude Code, Cursor, and more) from modifying files until required skills have been loaded in the current session. It works as a pre-tool-use hook that checks enforcement rules and blocks operations when required skills are missing.

## Quick Start

### Install

```bash
# Build from source
git clone https://github.com/Blue-Bear-Security/care-bare.git
cd care-bare && make install
```

### Run

```bash
# Launch from anywhere — shows all your projects across all agents
care-bare
```

care-bare discovers projects from Claude Code (`~/.claude/projects/`) and Cursor (`~/.cursor/projects/`) automatically. Select a project, then configure enforcement rules.

### Initialize a project

```bash
# From within a project directory
cd your-project
care-bare init
```

This creates `.care-bare/` with default configs, detects AI agents (Claude Code, Cursor), and installs hooks into their global config files.

### Or just configure rules directly

```bash
# Launch TUI, pick your project, configure rules
care-bare
```

No `init` required — you can configure rules for any project from the TUI.

## How It Works

```
AI Agent (Claude Code / Cursor)
    |
    | PreToolUse event (JSON via stdin)
    v
care-bare hook
    |
    +-- Parse input (adapter normalizes agent-specific format)
    +-- Check skill invocation --> Record & allow
    +-- Load enforcement rules from project's .care-bare/
    +-- Load session state (which skills are loaded)
    +-- Evaluate rules (ShouldBlock)
    |
    +-- Allowed --> agent proceeds
    +-- Blocked --> "Load skill by running: /git"
                    Agent loads the skill, retries, succeeds
```

### Skill Loading

- **Claude Code**: Agent runs `/skill-name` (native Skill tool) — care-bare records it
- **Cursor**: Agent reads `.claude/skills/skill-name/SKILL.md` — care-bare auto-detects and records it
- Both approaches ensure the agent actually reads the skill guidelines before proceeding

### Real-time Monitoring

The TUI shows which skills are loaded in which agent sessions, updating in real-time via filesystem watching. Skills loaded in Claude show a `claude` badge, skills loaded in Cursor show a `cursor` badge.

## Features

- **Machine-Level Project Discovery** — discovers all projects across all AI agents from anywhere
- **Interactive TUI Dashboard** — split-pane view: skills on the left, rules + inline editing on the right
- **Pre-Tool-Use Enforcement** — blocks file modifications until required skills are loaded
- **Multi-Agent Support** — ships with adapters for Claude Code and Cursor. Pluggable interface for adding more
- **Inline Rule Editing** — cycle tool/agent, edit path, duplicate, delete — all from the keyboard
- **File-Based State** — session skill tracking via filesystem with cross-process safety
- **Auto-Discovery** — scans configured paths for skill definitions (SKILL.md, .mdc files)
- **Event Logging** — every block, allow, and skill load is logged to `.care-bare/events.log`
- **Real-time Updates** — skill load badges update live via fsnotify when agents load skills
- **Zero Runtime Dependencies** — single static binary, no daemon, no background process

## Configuration

### `skill_enforcement.json`

```json
{
  "version": 1,
  "tools": [
    {
      "tool": "Edit",
      "path": "**/*.go",
      "skill": "go-coding-standards",
      "agent": "*"
    }
  ]
}
```

| Field | Description |
|-------|-------------|
| `version` | Schema version (currently `1`) |
| `tool` | Tool to match: `Edit`, `Write`, `Bash`, `Read`, `Glob`, `Grep`, `Agent`, `*` |
| `path` | Glob pattern for file paths (relative patterns auto-prefix with `**/`) |
| `skill` | Skill name that must be loaded before this tool+path is allowed |
| `agent` | Agent scope: `claude`, `cursor`, `*` (all) |

### `config.json`

```json
{
  "skill_paths": [".claude/skills"],
  "state_ttl_hours": 24,
  "default_agent": "*",
  "ignore_patterns": [".git", "node_modules", "vendor", "dist"]
}
```

See [docs/configuration.md](docs/configuration.md) for the full reference.

## Supported Agents

| Agent | Enforcement | Skill Detection | Hook Format |
|-------|------------|-----------------|-------------|
| Claude Code | Blocks via PreToolUse hook | Native `/skill` command | `hookSpecificOutput` JSON |
| Cursor | Blocks via preToolUse hook | Auto-detect SKILL.md reads | `{"continue": false}` + exit 2 |
| Custom | Add your own | Implement `HookAdapter` interface | — |

### Adding a New Agent

Implement the `HookAdapter` interface in `internal/adapter/`:

```go
type HookAdapter interface {
    Name() string
    ParseInput(stdin io.Reader) (*HookInput, error)
    FormatAllow() ([]byte, error)
    FormatDeny(reason string) ([]byte, error)
    ConfigPath() string
    InstallHook(projectDir string) error
    DetectSkillInvocation(input *HookInput) (skillName string, isSkill bool)
    ScanProjects() ([]AgentProject, error)
}
```

Register it in `registry.go` and you're done. The engine, TUI, and all commands work automatically.

## CLI Reference

| Command | Description |
|---------|-------------|
| `care-bare` | Launch interactive TUI with project picker |
| `care-bare init` | Initialize care-bare in the current project |
| `care-bare hook [--agent claude\|cursor]` | Run as a pre-tool-use hook (called by agents) |
| `care-bare status [--session <id>]` | Show rules, sessions, skills, and agent integrations |
| `care-bare clean [--all] [--session <id>]` | Clean up session state files |
| `care-bare doctor` | Check installation health |
| `care-bare version` | Print version information |

**Global flags:** `--config <path>` (override config), `--verbose` (debug logging)

## Architecture

```
care-bare/
  internal/
    adapter/     # Agent-specific logic (Claude, Cursor, future agents)
    cli/         # Cobra commands (hook, init, status, clean, doctor)
    engine/      # Core enforcement (ShouldBlock, config loading, glob matching)
    state/       # File-based session state with locking
    scanner/     # Skill discovery + project discovery
    tui/         # Charmbracelet TUI (dashboard, rule editor, tree picker)
```

All agent-specific logic lives in adapters. The engine is agent-agnostic.

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache 2.0 -- Copyright (c) Blue Bear Security. See [LICENSE](LICENSE).
