# Contributing to care-bare

Thank you for your interest in contributing to care-bare! This project is open source under the Apache 2.0 license, and we welcome contributions of all kinds: bug reports, feature suggestions, documentation improvements, and code.

## Reporting Bugs

Use [GitHub Issues](https://github.com/Blue-Bear-Security/care-bare/issues) to report bugs. Please include:

- **Go version:** output of `go version`
- **Operating system:** macOS, Linux, or Windows, with version
- **care-bare version:** output of `care-bare version`
- **Steps to reproduce:** minimal sequence of commands to trigger the bug
- **Expected behavior:** what you expected to happen
- **Actual behavior:** what actually happened, including any error messages or output

## Suggesting Features

Use [GitHub Issues](https://github.com/Blue-Bear-Security/care-bare/issues) with the `enhancement` label. Describe the use case you are trying to solve, not just the desired solution. This helps us understand the problem space and design the right approach.

## Development Setup

### Prerequisites

- **Go 1.22+** (care-bare uses Go 1.24 features but builds with 1.22+)
- **golangci-lint** (`brew install golangci-lint` or see [installation guide](https://golangci-lint.run/welcome/install/))
- **make**
- **VHS** (optional, for regenerating demo GIFs -- see [Charmbracelet VHS](https://github.com/charmbracelet/vhs))

### Clone and Build

```bash
git clone https://github.com/Blue-Bear-Security/care-bare.git
cd care-bare
make build
```

### Run Tests

```bash
make test
```

### Run Linter

```bash
make lint
```

### Regenerate Demo GIFs (optional)

```bash
make demos
```

This requires VHS to be installed. The tapes in `demo/` are recorded and output to `docs/assets/`.

## Pull Request Process

1. **Fork the repository** and create a feature branch from `main`.
2. **Write tests first** (TDD). All PRs must include tests that cover the new behavior or the bug being fixed.
3. **Run `make lint test`** before submitting. CI will run these checks, but catching issues locally saves time.
4. **Use conventional commit messages.** Examples:
   - `feat: add cursor adapter support`
   - `fix: handle empty config file gracefully`
   - `docs: update CLI reference for clean command`
   - `refactor: extract glob normalization to separate function`
   - `test: add table-driven tests for ShouldBlock`
5. **One logical change per PR.** If you are fixing a bug and adding a feature, submit them as separate PRs.
6. **PRs are reviewed** by at least one maintainer before merging.

## Code Style

- Follow `gofmt` formatting (enforced by `golangci-lint`).
- All exported types and functions must have doc comments.
- Use table-driven tests where there are multiple input/output cases.
- Use `t.TempDir()` for tests that need filesystem access.
- Use `t.Parallel()` where tests are safe to run concurrently.
- Keep functions focused: one function, one responsibility.
- Prefer returning errors over panicking.

## Testing Requirements

- **All new features** must have unit tests.
- **Bug fixes** must include a regression test that fails without the fix and passes with it.
- **CLI commands** should have integration-style tests using `cobra.Command` execution with captured stdout/stderr.
- **TUI components** should have tests using the `teatest` pattern where applicable.
- **File-system tests** should use `t.TempDir()` to avoid polluting the working directory.

## License

By contributing to care-bare, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE). All new files should not include individual copyright headers -- the repository-level LICENSE file covers all contributions.
