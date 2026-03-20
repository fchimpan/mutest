# mutest

[![CI](https://github.com/fchimpan/mutest/actions/workflows/ci.yml/badge.svg)](https://github.com/fchimpan/mutest/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/fchimpan/mutest)](https://goreportcard.com/report/github.com/fchimpan/mutest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Mutation testing for Go that finishes before your coffee cools.**

mutest targets only boundary-value comparison operators (`>`, `>=`, `<`, `<=`) — the #1 source of off-by-one errors. It runs in seconds, not minutes. Zero dependencies. Pure Go standard library.

```
$ mutest -v ./...
[KILLED  ] calc.go:13:11  > to >=   (632ms)
[SURVIVED] calc.go:5:7    > to >=   (207ms)   ← test gap found!

Killed: 1 (25.0%)  Survived: 3  Duration: 633ms
```

---

## Why mutest?

Traditional mutation testing tools mutate *everything*: arithmetic, logic, assignments, returns. The result? Thousands of mutants, hour-long runs, and noise that nobody reviews.

The reality is simpler: **most real-world bugs cluster around boundary conditions.** A `>` that should be `>=`. A `<` that should be `<=`. These are the mutations that matter.

| | Traditional tools | mutest |
|---|---|---|
| **Scope** | All operators | Comparison boundaries only |
| **Runtime** | Minutes to hours | **Seconds** |
| **Signal-to-noise** | Low (many trivial survivors) | **High** (survivors = real test gaps) |
| **CI-friendly** | Rarely | **By design** |

---

## Features

- **Boundary-value mutations** — `>` ↔ `>=`, `<` ↔ `<=`
- **Non-destructive** — Uses Go's `-overlay` flag; source files are never touched
- **Parallel execution** — Worker pool bounded by CPU cores
- **JSON output** — Machine-readable output for CI pipelines and AI agents (`-json`)
- **Dry-run mode** — Preview mutations without running tests (`-dry-run`)
- **Extensible** — `Mutator` interface for adding new mutation tiers
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

# Target a specific package with verbose output
mutest -v ./pkg/calc

# Only run tests matching a regex (like go test -run)
mutest -v -run TestBoundary ./...

# Tune parallelism and timeout
mutest -workers 4 -timeout 60s ./...

# Preview mutations without running tests
mutest -dry-run ./...

# JSON output for CI pipelines
mutest -json ./...
```

---

## Example Output

```
$ mutest -v ./...
mutest: discovered 4 mutation points
mutest: testing with 10 workers, 30s timeout per mutant

[SURVIVED] calc.go:5:7    > to >=   (207ms)
[SURVIVED] calc.go:21:7   > to >=   (211ms)
[SURVIVED] calc.go:18:7   < to <=   (211ms)
[KILLED  ] calc.go:13:11  > to >=   (632ms)

===== Mutation Testing Summary =====
Total:     4
Killed:    1 (25.0%)
Survived:  3
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
| **SURVIVED** | Your tests missed it — **a real test gap you should fix** |
| **ERROR** | Infrastructure failure (not counted in the score) |

**Mutation Score** = Killed / (Killed + Survived). Higher is better.

### Fixing a Survived Mutant

When mutest reports a survivor like this:

```
[SURVIVED] calc.go:5:7  > to >=
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

Re-run mutest and that mutation point will now show `[KILLED]`.

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | All mutants killed — boundary coverage is solid |
| `1` | Surviving mutants detected — test gaps found |
| `2` | Fatal error (e.g., project parse failure) |

---

## CLI Reference

```
mutest [flags] [packages]
```

Positional arguments are package patterns (default: `./...`), following the same conventions as `go test`.

| Flag | Default | Description |
|------|---------|-------------|
| `-v` | `false` | Print details for each mutant |
| `-json` | `false` | Emit results as JSON (NDJSON when combined with `-v`) |
| `-dry-run` | `false` | Discover mutations without running tests |
| `-run` | | Regexp to pass to `go test -run` |
| `-workers` | `NumCPU` | Max parallel test processes |
| `-timeout` | `30s` | Per-mutant test timeout |
| `-version` | | Print version and exit |

### JSON Output

The `-json` flag produces machine-readable output suitable for CI pipelines and AI agents.

**Summary mode** (`-json`): Emits a single JSON object with all results:

```bash
$ mutest -json ./...
{"total":4,"killed":1,"survived":3,"errors":0,"kill_rate":25,"duration":"633ms","results":[...]}
```

**Streaming mode** (`-json -v`): Emits one NDJSON line per mutant as results arrive, followed by a summary line:

```bash
$ mutest -json -v ./...
{"status":"killed","file":"calc.go","line":13,"column":11,"original":">","mutated":">=","desc":"> to >=","duration":"632ms"}
{"status":"survived","file":"calc.go","line":5,"column":7,"original":">","mutated":">=","desc":"> to >=","duration":"207ms"}
...
{"total":4,"killed":1,"survived":3,"errors":0,"kill_rate":25,"duration":"633ms","results":null}
```

When `-json` is active, informational messages are sent to stderr to keep stdout machine-parseable.

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
2. **Discover** — Walk the AST to find `ast.BinaryExpr` with `>`, `>=`, `<`, `<=`
3. **Mutate** — For each point, re-parse the file and swap the operator
4. **Overlay** — Write the mutated source to a temp file; generate `overlay.json` mapping `original.go → mutated.go`
5. **Test** — Run `go test -overlay=overlay.json ./...` in a parallel worker pool
6. **Judge** — `exit 0` = survived (test gap), `exit != 0` = killed (caught)

Go's [`-overlay` flag](https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies) tells the compiler "use this file instead of that one" without touching disk. Each mutant gets its own overlay, its own `go test` process, and its own goroutine. **Original source files are never modified.**

---

## CI Integration

### GitHub Actions

```yaml
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod

- name: Run mutation tests
  run: |
    go install github.com/fchimpan/mutest@latest
    mutest -v ./...
```

Surviving mutants cause exit code `1`, failing the CI step automatically.

Use `-json` for structured output that integrates with other tools:

```yaml
- name: Run mutation tests (JSON)
  run: |
    go install github.com/fchimpan/mutest@latest
    mutest -json ./... | tee mutation-report.json
    # Parse with jq, upload as artifact, etc.
```

---

## Architecture

```
mutest/
├── main.go              # CLI entry point, flags, reporting
├── mutator/
│   ├── mutator.go       # Mutator interface & MutationPoint type
│   └── comparison.go    # Tier 1: boundary comparison mutations
├── engine/
│   └── engine.go        # AST traversal, overlay generation, temp files
└── runner/
    └── runner.go        # Parallel test execution & result aggregation
```

### Extending mutest

Adding a new mutation tier requires only one file. Implement the `Mutator` interface:

```go
type Mutator interface {
    Name() string
    Discover(fset *token.FileSet, file *ast.File, filePath, pkg string) []MutationPoint
    Apply(file *ast.File, point MutationPoint)
}
```

Register it in `main.go`. No changes to engine or runner.

---

## Roadmap

- [x] JSON output (`-json`, NDJSON streaming with `-json -v`)
- [x] Dry-run mode (`-dry-run`)
- [ ] **Tier 2**: `==` ↔ `!=` mutations
- [ ] **Tier 3**: `&&` ↔ `||` mutations
- [ ] Coverage-based skip (don't test mutations on uncovered lines)
- [ ] JUnit report output

---

## Contributing

Contributions are welcome! Please open an issue first to discuss what you'd like to change.

```bash
git clone https://github.com/fchimpan/mutest.git
cd mutest
go test ./...                         # run tests
cd testdata/project && go run ../.. -v ./...  # try it out
```

## License

MIT
