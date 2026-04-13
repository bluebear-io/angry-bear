---
name: bug-fix
description: "Regression-first TDD bug fix workflow with real tests. Use when: fixing bugs, resolving regressions, addressing runtime errors, debugging failures, or when the user reports something broken. Triggers on words like: bug, broken, error, crash, regression, not working, fix."
---

# Bug Fix — Mandatory Sequence

Copy this checklist and check off items as you complete them. Do NOT skip or reorder steps.

```
Bug Fix Progress:
- [ ] Step 1: Write failing test (real filesystem, real execution, NO mocks)
- [ ] Step 2: Run test, confirm it FAILS
- [ ] Step 3: Show user the failing output
- [ ] Step 4: Write minimal fix
- [ ] Step 5: Run regression test, confirm it PASSES
- [ ] Step 6: Run full test suite + lint, confirm no regressions
- [ ] Step 7: Report summary
```

## Step 1: Write a failing test

Read 1-2 files to locate the bug entry point. Then IMMEDIATELY write a test.

**Tests must exercise real code paths:**
- CLI commands: use `cobra.Command` with captured stdout/stderr
- Engine logic: call the real function with real inputs
- State management: use `t.TempDir()` with real file I/O
- TUI: use `tea.KeyMsg` to simulate user input, verify model state
- Adapters: use real JSON input, verify parsed output

**Do NOT use mocks.** This project has zero mock dependencies. Tests use real filesystems, real JSON parsing, real enforcement logic.

**Test naming**: `TestBugFix_DescriptionOfBug`
**Test location**: same `_test.go` file as the code being fixed

**If you cannot reproduce in a test after 5 minutes, STOP and ask the user for a detailed scenario.**

## Step 2: Run test, confirm FAIL

```bash
go test -run TestBugFix_DescriptionOfBug ./internal/...
```

If the test passes, it does not reproduce the bug. Rewrite it.

## Step 3: Show failing output

Print the test failure to the user. Do NOT proceed to fixing until the user sees the reproduction.

## Step 4: Minimal fix

Change only what is necessary. No refactoring, no cleanup, no unrelated improvements.

## Step 5: Run regression test

```bash
go test -run TestBugFix_DescriptionOfBug ./internal/...
```

Must pass.

## Step 6: Full test suite + lint

```bash
make lint test
```

All tests must pass. Coverage must not drop below 80%.

## Step 7: Report

- Root cause (1 sentence)
- Test added (file path + test name)
- Code changed (file path + what changed)
- Results (regression passes, full suite passes, lint clean)

## Absolute rules — violations are unacceptable

1. NEVER write the fix before the failing test exists and has been shown to fail
2. NEVER use mocks — this project has zero mock dependencies
3. NEVER read more than 2 files before writing the test
4. NEVER explain or analyze at length before reproducing — reproduce FIRST
5. If you cannot reproduce in a test, STOP and ask the user for more context
