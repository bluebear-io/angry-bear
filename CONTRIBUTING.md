# Contributing to angry-bear

Thank you for your interest in contributing to angry-bear! This project is open source under the Apache 2.0 license, and we welcome contributions of all kinds.

## Reporting Bugs

Use [GitHub Issues](https://github.com/Blue-Bear-Security/angry-bear/issues). Please include:

- **Go version:** `go version`
- **OS:** macOS, Linux, or Windows with version
- **angry-bear version:** `angry-bear version`
- **Steps to reproduce**
- **Expected vs actual behavior**

## Suggesting Features

Use [GitHub Issues](https://github.com/Blue-Bear-Security/angry-bear/issues) with the `enhancement` label. Describe the **use case**, not just the solution.

## Development Setup

### Prerequisites

- **Go 1.22+**
- **golangci-lint** (`brew install golangci-lint`)
- **make**

### Build & Test

```bash
git clone https://github.com/Blue-Bear-Security/angry-bear.git
cd angry-bear
make build    # Build binary to bin/angry-bear
make test     # Run all tests with race detection
make lint     # Run linter
make install  # Install to $GOPATH/bin
```

## Pull Request Process

1. Fork the repository, create a feature branch from `main`
2. Write tests first (TDD) — all PRs must include tests
3. Run `make lint test` before submitting
4. Use conventional commit messages: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`
5. One logical change per PR
6. PRs are reviewed by at least one maintainer

## Code Style

- `gofmt` formatting (enforced by linter)
- All exported types and functions must have doc comments
- Table-driven tests for multiple input/output cases
- `t.TempDir()` for filesystem tests
- `t.Parallel()` where safe
- One function, one responsibility

## Architecture Rules

### All agent-specific logic lives in adapters

This is the most important rule. The engine, TUI, CLI commands, and state manager are **agent-agnostic**. They know nothing about Claude Code, Cursor, or any specific agent.

When you find yourself writing `if agent == "claude"` outside of `internal/adapter/`, you're doing it wrong. Put it in the adapter.

### The HookAdapter interface is the extension point

```go
type HookAdapter interface {
    Name() string
    ParseInput(stdin io.Reader) (*HookInput, error)
    FormatAllow() ([]byte, error)
    FormatDeny(reason string) ([]byte, error)
    ConfigPath() string
    InstallHook(projectDir string) error
    UninstallHook() error
    ExitCodeForDeny() int
    GlobalConfigPath() string
    DetectSkillInvocation(input *HookInput) (skillName string, isSkill bool)
    ScanProjects() ([]AgentProject, error)
}
```

Every method has a clear responsibility:

| Method | What it does |
|--------|-------------|
| `Name()` | Returns the adapter identifier (e.g., "codex") |
| `ParseInput()` | Reads agent-specific JSON from stdin, normalizes to `HookInput` |
| `FormatAllow()` | Formats the agent-specific "allow" response |
| `FormatDeny()` | Formats the agent-specific "deny/block" response |
| `ConfigPath()` | Returns the path to detect if this agent is present in a project |
| `InstallHook()` | Installs angry-bear hooks into the agent's global config file |
| `DetectSkillInvocation()` | Detects if the current tool call is a skill being loaded |
| `ScanProjects()` | Discovers all projects that have sessions with this agent |

## Adding a New Agent Adapter

This is the primary way the community extends angry-bear. Here's the complete guide:

### 1. Create the adapter file

Create `internal/adapter/myagent.go`:

```go
package adapter

import "io"

type MyAgentAdapter struct{}

func (a *MyAgentAdapter) Name() string { return "myagent" }

func (a *MyAgentAdapter) ParseInput(stdin io.Reader) (*HookInput, error) {
    // Read JSON from stdin
    // Extract: session ID, tool name, file path, working directory
    // Normalize tool names to canonical: Edit, Write, Bash, Read, Glob, Grep, Agent
    // Set Agent = "myagent"
    // Store raw JSON in RawInput for skill detection
}

func (a *MyAgentAdapter) FormatAllow() ([]byte, error) {
    // Return the JSON (or empty bytes) that tells the agent "proceed"
}

func (a *MyAgentAdapter) FormatDeny(reason string) ([]byte, error) {
    // Return the JSON that tells the agent "blocked" with the reason message
}

func (a *MyAgentAdapter) ConfigPath() string {
    // Return relative path to detect agent presence (e.g., ".myagent/config.json")
}

func (a *MyAgentAdapter) InstallHook(projectDir string) error {
    UninstallHook() error
    ExitCodeForDeny() int
    GlobalConfigPath() string
    // Add angry-bear hook to the agent's GLOBAL config file
    // Use resolveAngryBearCommand() for the absolute binary path
    // Preserve existing hooks — prepend, don't replace
    // Must be idempotent (safe to call twice)
}

func (a *MyAgentAdapter) DetectSkillInvocation(input *HookInput) (string, bool) {
    // If the agent has a native "load skill" tool, detect it here
    // Return (skillName, true) if detected
    // Return ("", false) otherwise
    // Note: SKILL.md reads are auto-detected in hook.go for ALL agents
}

func (a *MyAgentAdapter) ScanProjects() ([]AgentProject, error) {
    // Scan the agent's project directory (e.g., ~/.myagent/projects/)
    // Return AgentProject entries with decoded real paths
}
```

### 2. Register in the registry

In `internal/adapter/registry.go`, add to `NewRegistry()`:

```go
func NewRegistry() *AdapterRegistry {
    return &AdapterRegistry{
        adapters: map[string]HookAdapter{
            "claude":  &ClaudeAdapter{},
            "cursor":  &CursorAdapter{},
            "myagent": &MyAgentAdapter{},  // ADD THIS
        },
    }
}
```

### 3. Update auto-detection (if the agent sends unique JSON fields)

In `registry.go` `AutoDetect()`, add detection for your agent's unique JSON fields BEFORE the Claude fallback:

```go
if _, ok := parsed["myagent_version"]; ok {
    return r.Get("myagent")
}
```

### 4. Add tool name normalization

If your agent uses different tool names than angry-bear's canonical names, create a mapping in your adapter (see `cursorToolMap` in `cursor.go` for an example):

```go
var myAgentToolMap = map[string]string{
    "file.edit":   "Edit",
    "file.write":  "Write",
    "shell.exec":  "Bash",
    // ...
}
```

### 5. Write tests

Create `internal/adapter/myagent_test.go` with:
- ParseInput tests with fixture JSON
- FormatAllow/FormatDeny output tests
- InstallHook tests (create, preserve, idempotent)
- DetectSkillInvocation tests
- Test fixtures in `test/testdata/`

### 6. That's it

No other code needs to change. The engine, TUI, CLI commands, state manager, and project picker will automatically work with your new agent.

## Key Design Principles

1. **Adapters contain all agent knowledge** — engine is agent-agnostic
2. **Fail-open on config/state errors** — never block developers due to angry-bear bugs
3. **Fail-hard on user mistakes** — malformed JSON, bad config versions surface clearly
4. **File-based state** — no daemon, no background process, single static binary
5. **Hooks are global, rules are per-project** — install once, enforce per-project
6. **The CLI is observability** — hooks do the enforcement work

## Testing Requirements

- All new features must have unit tests
- Bug fixes must include a regression test
- CLI commands: integration tests with `cobra.Command` + captured stdout/stderr
- TUI: tests using Bubble Tea's `teatest` pattern
- Filesystem tests: always use `t.TempDir()`
- Use `HomeDir` and `BinaryPath` struct fields on adapters for test isolation (see adapter tests), or `SetRegistryDefaults` for CLI-level tests

## License

By contributing, you agree your work is licensed under [Apache License 2.0](LICENSE).
