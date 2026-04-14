---
name: pr-review
description: "Review PRs against care-bear architecture and coding guidelines. Use when: reviewing code, checking PRs, validating changes, auditing architecture compliance."
---

# PR Review — care-bear Guidelines

## Architecture Rules (NON-NEGOTIABLE)

### 1. All agent-specific logic lives in adapters
- `internal/adapter/` is the ONLY place for agent-specific code
- `if agent == "claude"` or `if agent == "cursor"` outside adapter/ = REJECT
- The engine, TUI, CLI commands, and state manager are agent-agnostic

### 2. HookAdapter interface is the extension point
Every piece of agent knowledge belongs in the adapter:
- `ExitCodeForDeny()` — not hardcoded in hook.go
- `GlobalConfigPath()` — not hardcoded in resolve.go
- `InstallHook()` / `UninstallHook()` — adapter manages its own hooks

### 3. No hardcoded agent names/paths outside adapters
- Badge health checks use the adapter registry, not hardcoded lists
- Hook installation iterates over registered adapters
- Config file detection uses `adapter.GlobalConfigPath()`

### 4. All data under ~/.care-bear/
- No project-level `.care-bear/` directories
- Config, state, logs all under `~/.care-bear/repos/{hash}/`
- `ResolveProjectRoot` uses `.git/` only (not `.care-bear/`)

## Code Standards

### From CLAUDE.md
- gofmt + golangci-lint must pass with 0 issues
- 80% minimum test coverage (CI enforced)
- No shorthand if — separate assignment from condition
- Octal format: `0o755` not `0755`
- Never swallow errors — wrap with context or log
- Table-driven tests, t.TempDir() for filesystem tests

### From CONTRIBUTING.md
- All exported types and functions must have doc comments
- One function, one responsibility
- Adapters contain ALL agent knowledge
- Fail-open on config/state errors (never block developers due to care-bear bugs)
- Fail-hard on user mistakes (malformed JSON, bad config versions)

## Review Checklist

- [ ] No agent-specific code outside `internal/adapter/`
- [ ] No hardcoded paths to `~/.claude/` or `~/.cursor/` outside adapters
- [ ] All new exported symbols have doc comments
- [ ] Tests included for new/changed code
- [ ] Coverage stays >= 80%
- [ ] No silent error swallowing (wrap or log)
- [ ] README/HIGHLEVEL updated if user-facing behavior changed
- [ ] HookAdapter interface updated if new adapter capability added
- [ ] ScrollView used for any scrollable list (no ad-hoc scroll logic)
- [ ] Shared constants in `tui/constants.go` (not duplicated)
