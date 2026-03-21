# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Design Philosophy

mutest is a focused mutation testing tool for Go that prioritizes **signal-to-noise ratio** over exhaustive coverage. It targets boundary-value comparison operators (`>`, `>=`, `<`, `<=`) and equality operators (`==`, `!=`) — the primary source of off-by-one and equality bugs. Do not propose adding exhaustive mutation strategies (arithmetic, assignments, returns, etc.) unless explicitly requested.

Key principles:
- **Speed over coverage** — runs in seconds, not minutes; must stay CI-friendly
- **High signal** — every surviving mutant should represent a real test gap
- **Non-destructive** — mutations are applied via Go's `-overlay` flag; source files are never modified

## Commands

```bash
go test ./... -race          # Run all tests
go test ./engine/ -run TestDiscover -race  # Run a single test
go vet ./...                 # Lint
go fix ./...                 # Modernize code
go build -o mutest .         # Build
```

## Architecture

Pipeline: **CLI (main.go) -> Engine -> Mutators -> Runner -> Report**

- **main.go** — CLI entry point. Parses flags, orchestrates the pipeline, formats output (text/JSON/NDJSON).
- **engine/engine.go** — Resolves packages via `go list -json`, parses source into ASTs, discovers mutation points using registered mutators, handles `//mutest:skip` directives, generates overlay files.
- **mutator/** — Interface-based plugin system (`Mutator` interface: `Discover` + `Apply`).
  - `comparison.go` — `>` <-> `>=`, `<` <-> `<=` (Tier 1)
  - `equality.go` — `==` <-> `!=` (Tier 2)
- **runner/runner.go** — Executes `go test -overlay=...` in a bounded worker pool with per-mutant timeouts.

To add a new mutator: implement the `Mutator` interface, then register it in `engine.New()`.

## Code Style

- Prefer table-driven tests
- Prefer real implementations over mocks where practical
- Run `go fix ./...` to keep code modernized
