# Configuration Reference

This document provides a complete reference for all care-bare configuration files, their fields, default values, and merge behavior.

## Config File Locations

| File | Location | Purpose |
|------|----------|---------|
| `skill_enforcement.json` | `~/.care-bare/skill_enforcement.json` | User-level enforcement rules (apply to all projects) |
| `skill_enforcement.json` | `.care-bare/skill_enforcement.json` | Project-level enforcement rules (shared via version control) |
| `config.json` | `.care-bare/config.json` | Project-level global settings (skill paths, TTL, ignore patterns) |
| `state/` | `.care-bare/state/` | Session state files (auto-managed, should be gitignored) |

User-level configs live in `~/.care-bare/`. Project-level configs live in `.care-bare/` at the project root. care-bare resolves the project root by walking up from the current directory looking for `.care-bare/`, then `.git/`, and falls back to the current directory itself.

---

## `skill_enforcement.json`

This file defines which skills must be loaded before specific tools can operate on specific file paths. It is the core configuration file for care-bare enforcement.

### Schema

```json
{
  "version": 1,
  "tools": [
    {
      "tool": "<tool-name>",
      "path": "<glob-pattern>",
      "skill": "<skill-name>",
      "agent": "<agent-name>"
    }
  ]
}
```

### Fields

#### `version` (int, required)

Config schema version. Must be `1`. Future versions may introduce breaking changes with migration tooling. care-bare returns an error for any version other than `1`.

#### `tools` (array of objects)

An array of enforcement rules. Each rule specifies a combination of tool, file path, skill, and agent that must be satisfied.

##### `tool` (string)

Tool name to match. Matching is exact and case-sensitive.

Valid values:
- `Edit` -- file editing tool
- `Write` -- file writing tool
- `Bash` -- shell command execution
- `Read` -- file reading tool
- `Glob` -- file search tool
- `Grep` -- content search tool
- `Agent` -- sub-agent invocation
- `Skill` -- skill loading (used internally for detection)
- `*` -- matches all tools
- `""` (empty string) -- matches all tools

##### `path` (string)

Glob pattern for file paths. Uses `doublestar` syntax that supports `**` for recursive directory matching.

- Relative patterns (not starting with `/` or `**/`) are automatically prefixed with `**/` so they match at any depth in the project tree.
- `*` or `""` (empty string) matches all files.
- File paths from agent input are normalized to project-root-relative, forward-slash format before matching.

Examples:
- `**/*.go` -- all Go files at any depth
- `*.go` -- normalized to `**/*.go` (same effect)
- `internal/**` -- all files under `internal/`
- `stacks/*.ts` -- normalized to `**/stacks/*.ts`
- `*` -- all files

##### `skill` (string, required)

Name of the skill that must have been invoked in the current session before this tool+path combination is allowed. This must match the skill name as discovered by the scanner (from YAML frontmatter or directory name).

##### `agent` (string)

Scope this rule to a specific agent. Matching is exact.

Valid values:
- `claude` -- applies only to Claude Code sessions
- `cursor` -- applies only to Cursor sessions
- `*` -- matches all agents
- `""` (empty string) -- matches all agents

### Examples

**Block all Go file edits without the go-coding skill:**

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

**Block all writes in the infrastructure directory without the infra skill:**

```json
{
  "version": 1,
  "tools": [
    {
      "tool": "Write",
      "path": "stacks/**",
      "skill": "sst-architect",
      "agent": "*"
    }
  ]
}
```

**Block all tools for all agents without the onboarding skill:**

```json
{
  "version": 1,
  "tools": [
    {
      "tool": "*",
      "path": "*",
      "skill": "onboarding",
      "agent": "*"
    }
  ]
}
```

**Scope a rule to Claude Code only:**

```json
{
  "version": 1,
  "tools": [
    {
      "tool": "Edit",
      "path": "**/*.py",
      "skill": "python-standards",
      "agent": "claude"
    }
  ]
}
```

---

## `config.json`

This file configures global care-bare behavior: where to find skills, how long session state lives, and which directories to ignore in the TUI.

### Schema

```json
{
  "skill_paths": ["<directory>"],
  "state_ttl_hours": 24,
  "default_agent": "*",
  "ignore_patterns": ["<directory-name>"]
}
```

### Fields

#### `skill_paths` (array of strings)

**Default:** `[".claude/skills"]`

Directories to scan for skill definitions. The scanner looks for `SKILL.md` files (Claude Code skills) and `.mdc` files (Cursor rules) within these directories and their subdirectories.

Relative paths are resolved from the project root. Absolute paths are used as-is.

```json
{
  "skill_paths": [".claude/skills", ".cursor/rules", "docs/skills"]
}
```

#### `state_ttl_hours` (int)

**Default:** `24`

Number of hours before session state files expire and become eligible for pruning. When a session state file's modification time exceeds this TTL, it is removed during the next prune cycle.

Pruning happens automatically (throttled to at most once per hour) after each hook invocation, or manually via `care-bare clean`.

#### `default_agent` (string)

**Default:** `"*"`

Default agent name used when auto-detection from stdin JSON fails and the `--agent` flag is not provided. In practice, auto-detection succeeds for Claude Code and Cursor, so this is a fallback for custom integrations.

#### `ignore_patterns` (array of strings)

**Default:** `[".git", "node_modules", "vendor", "dist", ".next", "__pycache__", ".venv", "build", "target"]`

Directory names to hide in the TUI tree picker when browsing for file paths. These are matched by directory name (not glob pattern) -- a value of `"node_modules"` hides any directory named `node_modules` at any depth.

---

## Config Merge Behavior

Rules from all config sources accumulate. care-bare loads configs in this order:

1. **User-level:** `~/.care-bare/skill_enforcement.json`
2. **Project-level:** Walk up from the current directory, collecting `.care-bare/skill_enforcement.json` files at each level, stopping at the filesystem root or user home directory.

All discovered rules apply simultaneously. There is no override or priority mechanism. If two rules match the same tool+path+agent combination but require different skills, both skills must be loaded before the operation is allowed.

Each rule tracks its source file path. You can see which file each rule came from via `care-bare status`.

---

## `.gitignore` Recommendations

The `care-bare init` command automatically adds `.care-bare/state/` to your `.gitignore`. If you set up care-bare manually, add this entry yourself:

```gitignore
# care-bare state (generated, do not commit)
.care-bare/state/
```

The following files **should be committed** to version control so enforcement rules are shared with the team:

- `.care-bare/skill_enforcement.json` -- enforcement rules
- `.care-bare/config.json` -- global settings

---

## Environment-Specific Notes

- **File locking:** Uses `gofrs/flock` for advisory file locks, which works on macOS, Linux, and Windows.
- **Path normalization:** File paths are normalized to forward slashes internally. Both forward slashes and backslashes in agent input are handled correctly.
- **State file permissions:** State files are created with `0600` permissions (owner read/write only).
- **Atomic writes:** State file writes use `natefinch/atomic` to prevent partial/corrupt files on crash.
