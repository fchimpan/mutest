# mutest

[![CI](https://github.com/fchimpan/mutest/actions/workflows/ci.yml/badge.svg)](https://github.com/fchimpan/mutest/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/fchimpan/mutest)](https://goreportcard.com/report/github.com/fchimpan/mutest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A mutation testing tool for Go. By default, mutest focuses on boundary-value and equality operators — a small but effective subset backed by research on [sufficient mutation operators](https://dl.acm.org/doi/10.1145/227607.227610). This keeps the mutant count low and execution fast.

```
$ mutest ./...
--- KILLED: calc.go:13:11  > to >= (0.63s)
--- SURVIVED: calc.go:5:7  > to >= (0.21s)   ← test gap found!

Killed: 1  Survived: 3  Score: 25.0%  Duration: 633ms
```

---

## Why mutest?

Mutation testing tools that mutate everything — arithmetic, logic, assignments, returns — generate thousands of mutants and take a long time to run. mutest takes a different approach: **focus on the operators that matter most and run fast.**

Relational Operator Replacement (ROR) — mutating `>`, `>=`, `<`, `<=`, `==`, `!=` — is one of the five [sufficient mutation operators](https://dl.acm.org/doi/10.1145/227607.227610) shown to achieve 99.5% of the fault detection of a full operator set. By limiting scope to ROR, mutest keeps the mutant count small enough to finish in seconds.

| | Full-scope tools | mutest |
|---|---|---|
| **Scope** | All operators | Boundary-value + equality |
| **Runtime** | Minutes to hours | **Seconds** |
| **Signal-to-noise** | Low (many trivial survivors) | **High** (survivors = real test gaps) |
| **CI-friendly** | Rarely | **By design** |

---

## Features

- **Boundary-value mutations** — `>` ↔ `>=`, `<` ↔ `<=`
- **Equality mutations** — `==` ↔ `!=`
- **Runtime instrumentation** — All mutations per package compiled into a single test binary; each mutant activated via `MUTEST_ID` env var at runtime, avoiding per-mutant recompilation
- **Non-destructive** — Source files are never modified; mutations are injected into generated code
- **Parallel execution** — Worker pool bounded by CPU cores
- **go test-style output** — `--- KILLED:` / `--- SURVIVED:` progress by default; `-v` shows test output like `go test -v`
- **Skip directive** — `//mutest:skip` to exclude functions or lines from mutation
- **Threshold gate** — `-threshold` flag for CI quality gates
- **False positive reduction** — Automatically skips `len(x) > 0` and similar no-op mutations
- **JSON output** — Machine-readable output for CI pipelines and AI agents (`-json`)
- **Dry-run mode** — Preview mutations without running tests (`-dry-run`)
- **Extensible** — `Mutator` interface for adding new mutation operators
- **Zero dependencies** — Go standard library only
- **CI-ready** — Exit code `1` on surviving mutants, `0` when all killed

---

## Installation

```bash
go install github.com/fchimpan/mutest@latest
```

Pre-built binaries for Linux, macOS, and Windows are available on the [Releases](https://github.com/fchimpan/mutest/releases) page.

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
| **ERROR** | Infrastructure failure (not counted in the score) |

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
| `1` | Surviving mutants detected (or kill rate below `-threshold`) |
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
| `-dry-run` | `false` | Discover mutations without running tests |
| `-run` | | Regexp to pass to `go test -run` |
| `-workers` | `NumCPU` | Max parallel test processes |
| `-timeout` | `30s` | Per-mutant test timeout |
| `-threshold` | `0` | Minimum kill rate % (0-100); exit 1 if below. 0 = any survived fails |
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

### Skip Directive

Use `//mutest:skip` comments to exclude specific functions or lines from mutation testing.

**Function-level skip** — add `//mutest:skip` as a doc comment to skip the entire function:

```go
//mutest:skip
func legacyCompare(a, b int) bool {
    return a > b // this mutation will be skipped
}
```

**Line-level skip** — add `//mutest:skip` as an inline comment to skip that line only:

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
3. **Instrument** — Replace each mutation target with a generic helper function call (e.g., `a > b` → `_mutest_cmp_1(a, b)`) and generate a runtime file that switches behavior based on `MUTEST_ID`
4. **Build** — Compile one test binary per package with all mutations embedded
5. **Test** — Run the pre-built binary once per mutation with `MUTEST_ID=N`, in a parallel worker pool
6. **Judge** — `exit 0` = survived (test gap), `exit != 0` = killed (caught), timeout = detected (hung)

This **runtime mutation selection** approach compiles each package only once regardless of how many mutations it contains. Traditional per-mutation compilation (`N mutations × compile`) is replaced with `P packages × compile + N mutations × run`, dramatically reducing overhead. **Original source files are never modified.**

### Skipped Mutations

mutest automatically skips mutations that are known to produce false positives:

| Pattern | Example | Why skipped |
|---------|---------|-------------|
| `len(x)` compared to `0` | `len(s) > 0` → `len(s) >= 0` | `len()` never returns negative, so the mutation cannot change behavior |
| `cap(x)` compared to `0` | `cap(s) > 0` → `cap(s) >= 0` | Same as `len()` — `cap()` is always non-negative |

These comparisons are collection-emptiness guards, not domain-logic boundaries. Mutating them always produces equivalent behavior, adding noise without signal.

Comparisons with non-zero literals (e.g., `len(s) > 1`) are **not** skipped — the boundary between 1 and 2 is meaningful.

---

## Architecture

```
mutest/
├── main.go              # CLI entry point, flags, reporting
├── mutator/
│   ├── mutator.go       # Mutator interface & MutationPoint type
│   ├── comparison.go    # Boundary comparison mutations (> >= < <=)
│   └── equality.go      # Equality mutations (== !=)
├── engine/
│   ├── engine.go        # AST traversal, overlay generation, //mutest:skip
│   └── instrument.go    # Runtime mutation instrumentation & test binary compilation
└── runner/
    └── runner.go        # Parallel test execution & result aggregation
```

---

## Contributing

Contributions are welcome! Please open an issue first to discuss what you'd like to change.

```bash
git clone https://github.com/fchimpan/mutest.git
cd mutest
go test ./...                         # run tests
cd testdata/project && go run ../.. ./...  # try it out
```

## License

MIT
