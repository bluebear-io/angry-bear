# care-bear — Agent Guidelines

## Overview

care-bear enforces skill-loading requirements for AI coding agents. Before modifying any code, read `docs/HIGHLEVEL.md` to understand how enforcement, state management, and the adapter architecture work.

## Repository Structure

```
internal/
  adapter/     Agent-specific logic (Claude, Cursor). ALL agent knowledge lives here.
  cli/         Cobra commands (hook, init, status, clean, doctor, version)
  engine/      Core enforcement (ShouldBlock, config loading, glob matching) — agent-agnostic
  state/       File-based session state with advisory locks and atomic writes
  scanner/     Skill discovery from configured paths
  tui/         Charmbracelet TUI (dashboard, rule editor, tree picker, settings)
cmd/care-bear/ Entry point — wires dependencies, nothing else
```

## Architecture Rules

1. **All agent-specific logic lives in adapters.** The engine, TUI, CLI, and state packages know nothing about Claude, Cursor, or any specific agent. If you write `if agent == "claude"` outside `internal/adapter/`, you're doing it wrong.

2. **The HookAdapter interface is the extension point.** See `internal/adapter/adapter.go`. New agents implement this interface and register in `registry.go`. Everything else works automatically.

3. **No global mutable state.** Adapters receive configuration via struct fields. No `init()` functions. No package-level mutable variables.

4. **Shared TUI components.** All scrollable lists use `ScrollView` from `internal/tui/scroll.go`. No ad-hoc scroll logic. `ToolOptions`, `AgentOptions`, `DefaultIgnoreSet` live in `internal/tui/constants.go`.

## Code Standards

This project follows strict Go coding standards:

- **gofmt + golangci-lint must pass** with zero issues
- **80% minimum test coverage** enforced by CI — every package must meet this individually
- **Table-driven tests** for functions with multiple input patterns
- **t.TempDir()** for all filesystem tests — never write to real paths
- **Never swallow errors** — wrap with context or log them
- **No shorthand if** — separate assignment from condition
- **Octal format** — use `0o755` not `0755`

## Testing

- Write failing tests FIRST (TDD), then implement
- Tests must validate real behavior, not just "it doesn't crash"
- UI tests are the most important — they test the user experience
- Use `tea.KeyMsg` to simulate key presses in TUI tests
- Run `go test -race -coverprofile=coverage.out ./...` before pushing

## Development

```bash
make build    # Build binary to bin/care-bear
make test     # Run all tests with race detection
make lint     # Run golangci-lint
make install  # Install to $GOPATH/bin
```

## Before Any Change

1. Run `make lint test` to confirm you're starting clean
2. Write a failing test for what you're about to change
3. Make the minimal change
4. Verify `make lint test` still passes
5. Check coverage hasn't dropped below 80%

## Documentation — Always Up to Date

**Every code change that adds, removes, or modifies user-facing behavior MUST update the relevant docs in the same PR:**

- **README.md** — CLI commands, install methods, feature descriptions, configuration reference
- **CLAUDE.md** — coding standards, architecture rules, if they change
- **docs/HIGHLEVEL.md** — architecture, enforcement flow, state management
- **CONTRIBUTING.md** — PR process, adapter guide, testing requirements
- **CLI --help text** — command descriptions, flag descriptions, examples

If you add a new command, it goes in README. If you change config format, it goes in README + HIGHLEVEL.md. If you change enforcement behavior, it goes in HIGHLEVEL.md. No exceptions — stale docs are worse than no docs.

## PR Requirements

- All CI checks must pass (Lint, Test, Build, GoReleaser Check)
- Coverage must stay at or above 80%
- Squash merge only
- One logical change per PR
