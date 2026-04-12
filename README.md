# care-bare

[![CI](https://github.com/Blue-Bear-Security/care-bare/actions/workflows/ci.yml/badge.svg)](https://github.com/Blue-Bear-Security/care-bare/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Blue-Bear-Security/care-bare)](https://goreportcard.com/report/github.com/Blue-Bear-Security/care-bare)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

**Enforce skill-loading requirements for AI coding agents.**

care-bare prevents AI coding agents (Claude Code, Cursor, etc.) from modifying files until required skills have been loaded in the current session. It works as a pre-tool-use hook that checks enforcement rules and blocks operations when required skills are missing.

<!-- Generate demo GIFs with 'make demos' -->

## Quick Start

### Install

```bash
# Homebrew (recommended)
brew install Blue-Bear-Security/tap/care-bare

# Or via Go (requires Go 1.22+, installs to $GOPATH/bin)
go install github.com/Blue-Bear-Security/care-bare/cmd/care-bare@latest

# Or build from source
git clone https://github.com/Blue-Bear-Security/care-bare.git
cd care-bare && make install
```

> Make sure `$GOPATH/bin` is on your `PATH`. If `care-bare` isn't found after install, add `export PATH="$PATH:$(go env GOPATH)/bin"` to your shell profile.

### Initialize

```bash
cd your-project
care-bare init
```

This creates `.care-bare/`, detects AI agents (Claude Code, Cursor), and installs hooks automatically.

### Configure Rules

```bash
# Interactive TUI
care-bare

# Or edit directly
vim .care-bare/skill_enforcement.json
```

## Features

- **Interactive TUI Dashboard** -- Visual configuration of enforcement rules with a Charmbracelet-powered terminal UI
- **Pre-Tool-Use Enforcement** -- Blocks file modifications until required skills are loaded
- **Multi-Agent Support** -- Ships with adapters for Claude Code and Cursor; pluggable interface for adding more
- **File-Based State** -- Session skill tracking via filesystem with cross-process safety (advisory locks, atomic writes)
- **Auto-Discovery** -- Scans configured paths for skill definitions (SKILL.md, .mdc files)
- **Zero Runtime Dependencies** -- Single static binary, no daemon, no background process

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
    },
    {
      "tool": "Write",
      "path": "infrastructure/**",
      "skill": "infra-review",
      "agent": "claude"
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

| Agent | Status | Config File |
|-------|--------|-------------|
| Claude Code | Supported | `.claude/settings.json` |
| Cursor | Supported | `.cursor/hooks.json` |
| Custom | Add your own | Implement `HookAdapter` interface |

## How It Works

```
AI Agent (Claude Code / Cursor)
    |
    | PreToolUse event (JSON via stdin)
    v
care-bare hook
    |
    +-- Parse input (adapter)
    +-- Check skill invocation --> Record & allow
    +-- Load enforcement rules (engine)
    +-- Load session state (state manager)
    +-- Evaluate rules (ShouldBlock)
    |
    +-- Allowed --> exit 0 (empty stdout)
    +-- Blocked --> exit 0 (deny JSON in stdout)
```

See [docs/architecture.md](docs/architecture.md) for the full deep dive.

## CLI Reference

| Command | Description |
|---------|-------------|
| `care-bare` | Launch interactive TUI dashboard |
| `care-bare init` | Initialize care-bare in the current project |
| `care-bare hook [--agent claude\|cursor]` | Run as a pre-tool-use hook (called by agents) |
| `care-bare status [--session <id>]` | Show rules, sessions, skills, and agent integrations |
| `care-bare clean [--all] [--session <id>]` | Clean up session state files |
| `care-bare doctor` | Check installation health |
| `care-bare version` | Print version information |

**Global flags:** `--config <path>` (override config), `--verbose` (debug logging)

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache 2.0 -- Copyright (c) Blue Bear Security. See [LICENSE](LICENSE).
