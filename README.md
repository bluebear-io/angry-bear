# care-bear

<p align="center">
  <img src="assets/logo.png" alt="care-bear" width="400">
</p>

[![CI](https://github.com/Blue-Bear-Security/care-bear/actions/workflows/ci.yml/badge.svg)](https://github.com/Blue-Bear-Security/care-bear/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/coverage-86%25-brightgreen)](https://github.com/Blue-Bear-Security/care-bear)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

**Enforce skill-loading requirements for AI coding agents.**

care-bear prevents AI coding agents (Claude Code, Cursor, and more) from modifying files until required skills have been loaded in the current session. It works as a pre-tool-use hook that checks enforcement rules and blocks operations when required skills are missing.

## Install

```bash
# Homebrew (recommended)
brew tap Blue-Bear-Security/care-bear
brew install care-bear

# Go (requires $GOPATH/bin on your PATH)
go install github.com/Blue-Bear-Security/care-bear/cmd/care-bear@latest

# From source
git clone https://github.com/Blue-Bear-Security/care-bear.git
cd care-bear && make install
# Binary installed to $GOPATH/bin — ensure it's on your PATH:
# export PATH="$HOME/go/bin:$PATH"  # add to ~/.zshrc or ~/.bashrc
```

## Quick Start

```bash
cd your-project
care-bear add go-standards --tool Edit,Write --path "**/*.go"
# ✓ Hooks installed for claude
# ✓ Hooks installed for cursor
# Added 2 rules for skill "go-standards"
```

Hooks auto-install on first use. When an agent tries to edit a Go file without loading `go-standards`:

```
Blocked by care-bear skill enforcement.
Required skills not loaded: "go-standards".
Load them by running: /go-standards
```

The agent loads the skill, retries, and succeeds.

## CLI Reference

### `care-bear add` — Add enforcement rules

```bash
care-bear add                    # Interactive mode — pick skill, tools, paths, agents
care-bear add <skill> [flags]    # One-liner — cartesian product of tools × paths × agents
```

| Flag | Default | Description |
|------|---------|-------------|
| `--tool` | `*` | Comma-separated: `Edit`, `Write`, `Bash`, `Read`, `Glob`, `Grep`, `Agent`, `*` |
| `--path` | `**` | Comma-separated glob patterns |
| `--agent` | `*` | `claude`, `cursor`, `*` (all) |

```bash
care-bear add sst-architect --tool Edit,Write --path "stacks/**"
care-bear add linear                                            # all tools, all paths
care-bear add testing --path "**/*_test.go,**/test_*.py"        # multiple patterns
```

### `care-bear rules` — List rules

```bash
care-bear rules                    # table format
care-bear rules --skill linear     # filter by skill
care-bear rules --json             # JSON for scripting
```

### `care-bear rm` — Remove rules

```bash
care-bear rm <skill>                           # all rules for a skill
care-bear rm go-standards --tool Bash           # specific matches only
care-bear rm testing --path "**/*_test.go"
```

### `care-bear enable` / `disable` — Hook management

```bash
care-bear enable     # Install hooks into Claude + Cursor configs
care-bear disable    # Remove ALL care-bear hooks (stops enforcement)
```

Rules are preserved when disabled. `enable` re-activates.

### `care-bear status` — Project overview

Shows enforcement rules, active sessions with loaded skills, discovered skill definitions, and detected agent integrations.

### `care-bear doctor` — Health check

Checks config validity, hook installation, state directory, binary on PATH, and skill paths. Each failure includes a fix hint.

### `care-bear clean` — Session cleanup

```bash
care-bear clean                    # remove expired sessions (TTL-based)
care-bear clean --all              # remove ALL sessions
care-bear clean --session <id>     # remove specific session
```

### `care-bear version`

```bash
care-bear version
# care-bear version v0.6.0 (commit: c9e9b85, built: 2026-04-13T16:29:05Z)
```

### `care-bear completion` — Shell completions

```bash
care-bear completion zsh > "${fpath[1]}/_care-bear"     # Zsh
care-bear completion bash > /etc/bash_completion.d/care-bear  # Bash
care-bear completion fish > ~/.config/fish/completions/care-bear.fish  # Fish
```

Tab-completes skill names, tool names, and agent names.

### Global Flags

| Flag | Description |
|------|-------------|
| `--config <path>` | Override config file path |
| `--verbose` | Debug logging to stderr |

## Interactive TUI

```bash
care-bear    # launches the TUI
```

### Dashboard Keys

| Key | Action |
|-----|--------|
| `↑↓` | Navigate within panel |
| `Tab` / `Shift+Tab` | Cycle panels: skills → rules → logs |
| `←→` | Switch panels |
| `Enter` / `a` | Add rules for selected skill |
| `t` | Cycle tool on selected rule |
| `p` | Edit path inline |
| `g` | Cycle agent on selected rule |
| `d` | Delete selected rule |
| `y` | Duplicate selected rule |
| `s` | Save to disk |
| `c` | Settings page |
| `P` | Switch project |
| `q` | Quit |

### Event Log Keys

| Key | Action |
|-----|--------|
| `f` | Filter mode (`←→` select column, `↑↓` cycle values) |
| `Esc` | Clear all filters |
| `PgUp/Dn` | Page through logs |
| `Home/End` | Jump to top/bottom |
| `Enter` | Jump to skill that caused event |

### Rule Editor Keys

| Key | Action |
|-----|--------|
| `↑↓` | Navigate within section |
| `Tab` | Next section (TOOLS → PATHS → AGENTS) |
| `Space` | Toggle selection |
| `→` / `Enter` | Expand directory |
| `←` | Collapse / parent |
| `s` | Save |
| `Esc` | Cancel |

### Settings Keys

| Key | Action |
|-----|--------|
| `↑↓` | Navigate |
| `←→` | Cycle values (checkout path) |
| `Enter` | Edit value |
| `g` / `p` | Switch global / project config |
| `Esc` | Save and exit |

## How It Works

```
AI Agent (Claude Code / Cursor)
    |
    | PreToolUse event (JSON via stdin)
    v
care-bear hook
    |
    +-- Parse input (adapter normalizes agent-specific format)
    +-- Check skill invocation → record in session state
    +-- Load enforcement rules
    +-- Load session state (check loaded skills + TTL)
    +-- Evaluate: ShouldBlock(rules, tool, path, agent, skills)
    |
    +-- Allowed → agent proceeds
    +-- Blocked → "Load skill by running: /skill-name"
```

### Data Storage

All data under `~/.care-bear/`. Nothing in project directories.

```
~/.care-bear/
  config.json                      # global defaults
  events.log                       # enforcement log
  repos/{hash}-{slug}/             # per-repo
    skill_enforcement.json         #   rules
    config.json                    #   config overrides
    state/{session}.json           #   loaded skills
    preferences.json               #   preferred checkout
```

See [docs/HIGHLEVEL.md](docs/HIGHLEVEL.md) for the complete architecture.

## Configuration

### `skill_enforcement.json`

```json
{
  "version": 1,
  "tools": [
    { "tool": "Edit", "path": "**/*.go", "skill": "go-standards", "agent": "*" }
  ]
}
```

### `config.json`

| Field | Default | Description |
|-------|---------|-------------|
| `skill_paths` | `[".claude/skills"]` | Dirs to scan for skills |
| `skill_ttl_minutes` | `0` | Skill expiry (0 = never) |
| `state_ttl_hours` | `24` | Session state retention |
| `default_agent` | `*` | Default agent for new rules |

## Supported Agents

| Agent | Hook Config | Deny Format |
|-------|------------|-------------|
| Claude Code | `~/.claude/settings.json` | Exit 0 + JSON |
| Cursor | `~/.cursor/hooks.json` | Exit 2 + JSON |
| Custom | Implement `HookAdapter` | Your format |

## Troubleshooting

### `care-bear: command not found`

```bash
export PATH="$HOME/go/bin:$PATH"  # add to ~/.zshrc
```

### Hook not firing?

```bash
care-bear doctor        # check "Hook installed" lines
care-bear enable        # reinstall hooks
```

Check `~/.claude/settings.json` → `hooks.PreToolUse` for a care-bear entry. Note: `settings.local.json` permission entries are NOT hooks.

### Rules not showing?

Run from within the project directory (needs `.git/` to identify the repo).

### Agent blocked but shouldn't be?

```bash
care-bear status        # check loaded skills
care-bear clean --all   # reset sessions
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). All CI checks must pass, 80% minimum coverage.

## License

Apache 2.0 — Copyright (c) Blue Bear Security. See [LICENSE](LICENSE).
