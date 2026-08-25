# mutest

[![CI](https://github.com/fchimpan/mutest/actions/workflows/ci.yml/badge.svg)](https://github.com/fchimpan/mutest/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/fchimpan/mutest)](https://goreportcard.com/report/github.com/fchimpan/mutest)
[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.24-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A mutation testing tool for Go. By default, mutest focuses on boundary-value and equality operators. This keeps the mutant count low and execution fast.

```
$ mutest ./...
--- KILLED: calc.go:13:11  > to >= (0.63s)
--- SURVIVED: calc.go:5:7  > to >= (0.21s)   ← test gap found!

Killed: 1  Survived: 3  Score: 25.0%  Duration: 633ms
```

---

## What is Mutation Testing?

Mutation testing evaluates the quality of your test suite by introducing small changes (**mutations**) to your source code and checking whether your tests detect them.

| Term | Meaning |
|------|---------|
| **Mutant** | A copy of your code with a single deliberate change (e.g., `>` replaced with `>=`) |
| **Killed** | A test failed after the mutation was applied — your tests caught the bug |
| **Survived** | All tests still passed after the mutation — **your tests have a gap** |
| **Timeout** | The mutation caused tests to hang or exceed the time limit — treated as detected |
| **Mutation Score** | `(Killed + Timeout) / (Killed + Timeout + Survived)` — higher is better |

A high mutation score means your tests are good at catching real bugs. Survived mutants point you to exact lines where adding a boundary or equality test would improve coverage.

---

## Why mutest?

Mutation testing tools that mutate everything — arithmetic, logic, assignments, returns — generate thousands of mutants and take a long time to run. mutest takes a different approach: **focus on the operators that matter most and run fast.**

Relational Operator Replacement (ROR) — mutating `>`, `>=`, `<`, `<=`, `==`, `!=` — is a well-studied subset of mutation operators known to be effective for fault detection. By limiting scope to ROR, mutest keeps the mutant count small enough to finish in seconds.

---

## Features

- **Fast** — One build per package, all mutants activated at runtime. No per-mutant recompilation
- **Parallel execution** — Worker pool bounded by CPU cores
- **`go test`-compatible** — `--- KILLED:` / `--- SURVIVED:` output; `-v`, `-run`, `-timeout` work as expected
- **`//mutest:skip`** — Exclude functions, blocks (`if`/`for`/`switch`/`select`), or individual lines from mutation
- **`-diff`** — Only mutate lines changed relative to a git ref (e.g., `-diff origin/main`)
- **`-threshold`** — CI quality gate (e.g., `-threshold 80` fails if score < 80%)
- **`-json`** — Machine-readable output for CI pipelines (`-json -v` for NDJSON streaming)
- **`-dry-run`** — Preview mutations without running tests
- **Low noise** — Automatically skips equivalent mutants (`len(x) > 0`) and simple error propagation (`if err != nil { return err }`)
- **Zero dependencies** — Go standard library only

---

## Installation

```bash
go install github.com/fchimpan/mutest@latest
```

Pre-built binaries for Linux, macOS, and Windows are available on the [Releases](https://github.com/fchimpan/mutest/releases) page.

### Requirements

The **target module** — the code you run mutest against — must declare `go 1.20` or later in its `go.mod`; mutest's generated comparison helpers use generics. mutest checks this before instrumenting anything and fails fast with a clear error if the module's `go` directive is too old, instead of an obscure compiler error. Modules with no reported Go version (e.g. GOPATH mode) are not checked.

---

## Quick Start

```bash
# Run against all packages (like go test ./...)
mutest ./...

# Target a specific package
mutest ./pkg/calc

# Show test output for each mutant (like go test -v)
mutest -v ./pkg/calc

# Only run tests matching a regex (like go test -run)
mutest -run TestBoundary ./...

# Tune parallelism and timeout
mutest -workers 4 -timeout 60s ./...

# Only mutate lines changed vs main (ideal for CI)
mutest -diff origin/main ./...

# CI quality gate: fail if kill rate is below 80%
mutest -threshold 80 ./...

# Preview mutations without running tests
mutest -dry-run ./...

# JSON output for CI pipelines
mutest -json ./...
```

---

## Example Output

```
$ mutest ./...
mutest: discovered 4 mutation points
mutest: testing with 10 workers, 30s timeout per mutant

--- SURVIVED: calc.go:5:7  > to >= (0.21s)
--- SURVIVED: calc.go:21:7  > to >= (0.21s)
--- SURVIVED: calc.go:18:7  < to <= (0.21s)
--- KILLED: calc.go:13:11  > to >= (0.63s)

===== Mutation Testing Summary =====
Total:     4
Killed:    1
Survived:  3
Score:     25.0%
Duration:  633ms

Survived mutants (test gaps):
  1. calc.go:5:7  > to >=
  2. calc.go:18:7  < to <=
  3. calc.go:21:7  > to >=
```

### Reading the Results

| Status | What it means |
|--------|---------------|
| **KILLED** | Your tests caught the mutation — the boundary is well-tested |
| **TIMEOUT** | The mutation caused tests to hang — counted as detected |
| **SURVIVED** | Your tests missed it — **a real test gap you should fix** |
| **ERROR** | Infrastructure failure, e.g. the test binary could not be run (not counted in the score) |
| **CANCELED** | The run was interrupted before this mutant was tested (not counted in the score) |

**Mutation Score** = (Killed + Timeout) / (Killed + Timeout + Survived). Higher is better.

> **Note:** Mutations that cause a panic (e.g., index out of bounds) are counted as **KILLED**. Go's exit code does not distinguish panics from test assertion failures, so mutest treats both as detected mutations.

### Fixing a Survived Mutant

When mutest reports a survivor like this:

```
--- SURVIVED: calc.go:5:7  > to >= (0.21s)
```

It means mutest swapped the operator and **no test noticed**:

```go
func Max(a, b int) int {
    if a > b {  // ← mutest changed this to >=, tests still passed
        return a
    }
    return b
}
```

The fix — add a test at the boundary:

```go
func TestMax_EqualValues(t *testing.T) {
    // This test kills the > → >= mutation because Max(3,3)
    // returns 3 with >, but would return a (also 3) with >=.
    // More importantly, it verifies the boundary behavior is intentional.
    if got := Max(3, 3); got != 3 {
        t.Errorf("Max(3,3) = %d, want 3", got)
    }
}
```

Re-run mutest and that mutation point will now show `--- KILLED:`.

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | All mutants killed (or kill rate meets `-threshold`) |
| `1` | Surviving mutants detected, kill rate below `-threshold`, a mutant errored, the baseline failed (tests fail without any mutation), or the run was interrupted |
| `2` | Fatal error (e.g., project parse failure) |

---

## CLI Reference

```
mutest [flags] [packages]
```

Positional arguments are package patterns (default: `./...`), following the same conventions as `go test`.

| Flag | Default | Description |
|------|---------|-------------|
| `-v` | `false` | Show test output for each mutant |
| `-json` | `false` | Emit results as JSON (NDJSON when combined with `-v`) |
| `-diff` | | Only mutate lines changed relative to this git ref (e.g., `origin/main`) |
| `-dry-run` | `false` | Discover mutations without running tests |
| `-run` | | Regexp to pass to `go test -run` |
| `-workers` | `NumCPU` | Max parallel test processes |
| `-timeout` | `30s` | Per-mutant test timeout |
| `-threshold` | `0` | Minimum kill rate % (0-100); exit 1 if below. 0 = any survived fails |
| `-skip-err-propagation` | `true` | Skip simple error propagation patterns (`if err != nil { return err }`) |
| `-version` | | Print version and exit |

### JSON Output

The `-json` flag produces machine-readable output suitable for CI pipelines and AI agents.

**Summary mode** (`-json`): Emits a single JSON object with all results:

```bash
$ mutest -json ./...
{"total":4,"killed":1,"timed_out":0,"survived":3,"errors":0,"kill_rate":25,"duration":"633ms","results":[...]}
```

**Streaming mode** (`-json -v`): Emits one NDJSON line per mutant as results arrive, followed by a summary line:

```bash
$ mutest -json -v ./...
{"status":"killed","file":"calc.go","line":13,"column":11,"original":">","mutated":">=","desc":"> to >=","duration":"632ms"}
{"status":"survived","file":"calc.go","line":5,"column":7,"original":">","mutated":">=","desc":"> to >=","duration":"207ms"}
...
{"total":4,"killed":1,"timed_out":0,"survived":3,"errors":0,"kill_rate":25,"duration":"633ms","results":null}
```

When `-json` is active, informational messages are sent to stderr to keep stdout machine-parseable.

### Diff Mode

The `-diff` flag restricts mutation testing to lines changed relative to a git ref. This makes mutest practical for CI — instead of setting a threshold over the entire codebase, you enforce that **new and changed code** is well-tested.

```bash
# Lines changed vs main branch
mutest -diff main ./...

# Remote main (safer in CI where local main may be stale)
mutest -diff origin/main ./...

# Last N commits
mutest -diff HEAD~3 ./...

# Specific commit or tag
mutest -diff v1.2.0 ./...
```

Internally, mutest resolves the merge-base of `<ref>` and `HEAD`, then runs `git diff --unified=0 <merge-base>` against the **working tree** to identify changed lines and filters mutation points to those locations. Comparing against the working tree means uncommitted edits (staged or unstaged) are included and line numbers match the sources on disk, so the filter stays accurate while you iterate locally. Untracked `.go` files are treated as fully changed. On a clean checkout (e.g. CI), the result is identical to a `<ref>...HEAD` merge-base diff.

```
$ mutest -diff origin/main ./...
mutest: diff mode: filtered to 5 of 42 mutation points (changed vs origin/main)
mutest: discovered 5 mutation points
mutest: testing with 10 workers, 30s timeout per mutant

--- KILLED: handler.go:25:11  > to >= (0.42s)
--- KILLED: handler.go:31:7   == to != (0.38s)
--- SURVIVED: handler.go:44:9  < to <= (0.19s)

===== Mutation Testing Summary =====
Total:     5
Killed:    4
Survived:  1
Score:     80.0%
Duration:  423ms
```

Combine with `-threshold 100` to require all changed comparisons to be covered:

```bash
mutest -diff origin/main -threshold 100 ./...
```

If the diff contains no mutation targets (e.g., only comments or non-Go files were changed), mutest exits 0.

### Skip Directive

Use `//mutest:skip` comments to exclude specific functions or lines from mutation testing.

**Function-level skip** — add `//mutest:skip` as a doc comment to skip the entire function:

```go
//mutest:skip
func legacyCompare(a, b int) bool {
    return a > b // this mutation will be skipped
}
```

**Block-level skip** — add `//mutest:skip` as an inline comment on an `if`, `for`, `switch`, or `select` statement to skip the entire block:

```go
func fetch(url string) (*Response, error) {
    resp, err := http.Get(url)
    if err != nil { //mutest:skip
        if errors.Is(err, context.Canceled) {
            return nil, ErrCanceled
        }
        return nil, fmt.Errorf("fetch: %w", err)
    }
    return resp, nil
}
```

**Line-level skip** — add `//mutest:skip` as an inline comment on any other line to skip that line only:

```go
func compare(a, b int) int {
    if a > b { //mutest:skip
        return 1
    }
    if a < b { // this mutation will NOT be skipped
        return -1
    }
    return 0
}
```

**Standalone-line skip** — add `//mutest:skip` alone on its own line (nothing but whitespace before it) to skip the line that follows — or the entire block, if that following line begins one (`if`/`for`/`switch`/`select`):

```go
func compare(a, b int) int {
    //mutest:skip
    if a > b {
        return 1 // part of the skipped block
    }
    //mutest:skip
    result := a < b // this mutation will be skipped too
    return boolToInt(result)
}
```

> **Note:** When `//mutest:skip` is placed on a block statement (`if`/`for`/`switch`/`select`), it skips the entire block including nested statements. At the end of any other line, it skips only that line. Placed alone on its own line, it skips the line immediately following it (the whole block, if that line opens one) — it never affects its own comment-only line, since there is no code there to skip.

### Dry-Run Mode

The `-dry-run` flag lists discovered mutation points without executing tests. Useful for previewing scope or counting mutations.

```bash
$ mutest -dry-run ./...
mutest: discovered 4 mutation points (dry run)

  1. calc.go:5:7  > to >=
  2. calc.go:13:11  > to >=
  3. calc.go:18:7  < to <=
  4. calc.go:21:7  > to >=
```

Combine with `-json` for machine-readable output:

```bash
$ mutest -dry-run -json ./...
[
  {"file":"calc.go","package":"testproject","line":5,"column":7,"original":">","mutated":">=","desc":"> to >="},
  ...
]
```

---

## How It Works

1. **Parse** — `go/parser` builds an AST from every non-test `.go` file
2. **Discover** — Walk the AST to find `ast.BinaryExpr` with `>`, `>=`, `<`, `<=`, `==`, `!=` (respecting `//mutest:skip`)
3. **Instrument** — Replace each ordered comparison with a generic helper call (e.g., `a > b` → `_mutest_cmp_1(a, b)`) and each equality comparison with a flip of its result (`a == b` → `(a == b) != _mutest_on(1)`; the original comparison stays in place because its operands may legally have different static types, e.g. `any == error`), then generate a runtime file that switches behavior based on `MUTEST_ID`
4. **Build** — Compile one test binary per package with all mutations embedded
5. **Verify baseline** — Run each test binary once with **no** mutation active. If any package's tests fail without a mutation, mutest aborts (a broken or flaky suite would otherwise make every mutant a false KILLED)
6. **Test** — Run the pre-built binary once per mutation with `MUTEST_ID=N`, in a parallel worker pool. Each binary runs with its package directory as the working directory, so `testdata` relative paths resolve
7. **Judge** — `exit 0` = survived (test gap), `exit != 0` = killed (caught), timeout = detected (hung)

A package with **no test files** builds no binary; its mutants are reported as `SURVIVED` (no tests is the largest possible test gap). If the run is interrupted (e.g. `Ctrl+C`), untested mutants are reported as `CANCELED` and mutest exits non-zero rather than judging success on partial results.

This **runtime mutation selection** approach compiles each package only once regardless of how many mutations it contains. Traditional per-mutation compilation (`N mutations × compile`) is replaced with `P packages × compile + N mutations × run`, dramatically reducing overhead. **Original source files are never modified.**

### Skipped Mutations

mutest automatically skips mutations that are known to produce false positives:

| Pattern | Example | Why skipped |
|---------|---------|-------------|
| `len(x)` compared to `0` | `len(s) > 0` → `len(s) >= 0` | `len()` never returns negative, so the mutation cannot change behavior |
| `cap(x)` compared to `0` | `cap(s) > 0` → `cap(s) >= 0` | Same as `len()` — `cap()` is always non-negative |
| Simple error propagation | `if err != nil { return err }` | Go's idiomatic error propagation — mutating these generates noise without meaningful test gaps |

Comparisons with non-zero literals (e.g., `len(s) > 1`) are **not** skipped — the boundary between 1 and 2 is meaningful.

Complex error handling is **not** skipped — compound conditions (`err != nil && !timedOut`), fallback assignments, and multi-statement bodies represent real logic that should be tested.

To disable error propagation skipping and mutate all `err != nil` checks:

```bash
mutest -skip-err-propagation=false ./...
```

---

## License

MIT
