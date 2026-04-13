# care-bare

<p align="center">
  <img src="assets/logo.png" alt="care-bare" width="400">
</p>

[![CI](https://github.com/Blue-Bear-Security/care-bare/actions/workflows/ci.yml/badge.svg)](https://github.com/Blue-Bear-Security/care-bare/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/coverage-86%25-brightgreen)](https://github.com/Blue-Bear-Security/care-bare)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/Blue-Bear-Security/care-bare)](https://goreportcard.com/report/github.com/Blue-Bear-Security/care-bare)

**Enforce skill-loading requirements for AI coding agents.**

care-bare prevents AI coding agents (Claude Code, Cursor, and more) from modifying files until required skills have been loaded in the current session. It works as a pre-tool-use hook that checks enforcement rules and blocks operations when required skills are missing.

## Install

```bash
# Homebrew
brew tap Blue-Bear-Security/care-bare
brew install care-bare

# Go
go install github.com/Blue-Bear-Security/care-bare/cmd/care-bare@latest

# From source
git clone https://github.com/Blue-Bear-Security/care-bare.git
cd care-bare && make install
```

## Quick Start

```bash
# Add enforcement rules — hooks auto-install on first use
cd your-project
care-bare add    # Interactive mode — pick skill, tools, paths, agents

# Or one-liner
care-bare add go-standards --tool Edit,Write --path "**/*.go"

# That's it — agents are now enforced
```

When an AI agent tries to edit a Go file without loading `go-standards`:

```
Blocked by care-bare skill enforcement.
Required skills not loaded: "go-standards".
Load them by running: /go-standards
```

The agent loads the skill, retries, and succeeds.

## CLI Commands

### Managing Rules

```bash
# Add rules (cartesian product of tools × paths × agents)
care-bare add <skill> [--tool Edit,Write] [--path "**/*.go"] [--agent claude]

# List all rules (or filter by skill)
care-bare rules [--skill <name>] [--json]

# Remove rules
care-bare rm <skill> [--tool Edit] [--path "**/*.go"]
```

**Examples:**

```bash
# Require "sst-architect" before editing any stack file
care-bare add sst-architect --tool Edit,Write --path "stacks/**"

# Require "linear" for all edits by all agents
care-bare add linear

# List rules as JSON (for scripting)
care-bare rules --json

# Remove all rules for a skill
care-bare rm old-skill

# Remove only specific rules
care-bare rm go-standards --tool Bash --path "scripts/**"
```

### Interactive TUI

```bash
# Launch the dashboard — discovers all projects across all agents
care-bare
```

The TUI provides:
- **Split-pane dashboard** — skills on the left, rules + event log on the right
- **Rule editor** — three pinned sections (Tools, Paths, Agents) with independent scrolling
- **Multi-column event log filter** — filter by action, project, session, agent, tool, skill
- **Real-time updates** — skill loads and enforcement events appear live
- **Settings page** — configure skill TTL, session TTL, switch checkouts (`c` key)

### Project Management

```bash
care-bare status        # Show rules, sessions, skills, agent integrations
care-bare doctor        # Check installation health
care-bare clean         # Clean expired session state
care-bare clean --all   # Remove all session state
care-bare version       # Print version info
```

### Shell Completions

```bash
# Zsh
care-bare completion zsh > "${fpath[1]}/_care-bare"

# Bash
care-bare completion bash > /etc/bash_completion.d/care-bare

# Fish
care-bare completion fish > ~/.config/fish/completions/care-bare.fish
```

Tab-complete skill names, tool names, and agent names.

**Global flags:** `--config <path>` (override config), `--verbose` (debug logging)

## How It Works

```
AI Agent (Claude Code / Cursor)
    |
    | PreToolUse event (JSON via stdin)
    v
care-bare hook
    |
    +-- Parse input (adapter normalizes agent-specific format)
    +-- Check skill invocation → record in session state
    +-- Load enforcement rules
    +-- Load session state (which skills are loaded, check TTL)
    +-- Evaluate: ShouldBlock(rules, tool, path, agent, skills)
    |
    +-- Allowed → agent proceeds
    +-- Blocked → "Load skill by running: /skill-name"
                   Agent loads the skill, retries, succeeds
```

### Skill Loading

- **Claude Code**: Agent runs `/skill-name` (native Skill tool) — care-bare records it
- **Cursor**: Agent reads `.claude/skills/skill-name/SKILL.md` — care-bare auto-detects and records it

### Skill TTL

Skills can expire after a configurable time, forcing agents to re-read guidelines:

```bash
# Set skill TTL to 60 minutes (0 = no expiry)
# In config.json or via the TUI settings page (c key)
{ "skill_ttl_minutes": 60 }
```

### Project Identity

Projects are identified by Git repository (not directory path). The same repo checked out in multiple locations is treated as one project. Config is stored at `~/.care-bare/repos/{hash}/`.

See [docs/HIGHLEVEL.md](docs/HIGHLEVEL.md) for the complete architecture documentation.

## Configuration

### `skill_enforcement.json`

```json
{
  "version": 1,
  "tools": [
    { "tool": "Edit", "path": "**/*.go", "skill": "go-standards", "agent": "*" },
    { "tool": "Write", "path": "stacks/**", "skill": "sst-architect", "agent": "claude" }
  ]
}
```

| Field | Description |
|-------|-------------|
| `tool` | `Edit`, `Write`, `Bash`, `Read`, `Glob`, `Grep`, `Agent`, `*` |
| `path` | Glob pattern for file paths |
| `skill` | Skill that must be loaded before this tool+path is allowed |
| `agent` | `claude`, `cursor`, `*` (all) |

### `config.json`

```json
{
  "skill_paths": [".claude/skills"],
  "skill_ttl_minutes": 0,
  "state_ttl_hours": 24,
  "default_agent": "*",
  "ignore_patterns": [".git", "node_modules", "vendor", "dist"]
}
```

Two tiers: `~/.care-bare/config.json` (global defaults) and `{project}/.care-bare/config.json` (project overrides).

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

## Architecture

```
care-bare/
  internal/
    adapter/     # Agent-specific logic (Claude, Cursor, future agents)
    cli/         # Cobra commands (hook, init, status, clean, doctor, add, rules, rm)
    engine/      # Core enforcement (ShouldBlock, config loading, glob matching)
    state/       # File-based session state with locking
    scanner/     # Skill discovery from configured paths
    tui/         # Charmbracelet TUI (dashboard, rule editor, settings, tree picker)
  cmd/care-bare/ # Entry point
  assets/        # Logo and static assets
```

All agent-specific logic lives in adapters. The engine is agent-agnostic. See [docs/HIGHLEVEL.md](docs/HIGHLEVEL.md) for the complete architecture guide.

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

**Requirements:**
- All CI checks must pass (Lint, Test, Build, GoReleaser Check)
- Minimum 80% test coverage (enforced by CI)
- Squash merge, one logical change per PR

## License

Apache 2.0 — Copyright (c) Blue Bear Security. See [LICENSE](LICENSE).
