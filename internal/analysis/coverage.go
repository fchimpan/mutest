package analysis

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// CoverageMap maps source file lines to the tests that cover them.
type CoverageMap struct {
	// key: "absolute/path/to/file.go:line" → test function names
	lineTests map[string][]string
	// key: "absolute/path/to/file.go:line" → true if covered
	covered map[string]bool
}

// BuildCoverageMap runs `go test -coverprofile` and builds a mapping of
// which source lines are covered by any test.
func BuildCoverageMap(dir string, patterns []string) (*CoverageMap, error) {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("mutest-cover-%d.prof", os.Getpid()))
	defer os.Remove(tmpFile)

	args := []string{"test", "-coverprofile", tmpFile, "-count=1"}
	args = append(args, patterns...)

	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr // show test output on stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("running coverage: %w", err)
	}

	return parseCoverProfile(tmpFile, dir)
}

// parseCoverProfile reads a Go cover profile and extracts coverage information.
// Cover profile format: "pkg/file.go:startLine.startCol,endLine.endCol numStmt count"
func parseCoverProfile(path string, moduleDir string) (*CoverageMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening cover profile: %w", err)
	}
	defer f.Close()

	cm := &CoverageMap{
		lineTests: make(map[string][]string),
		covered:   make(map[string]bool),
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip mode line
		if strings.HasPrefix(line, "mode:") {
			continue
		}
		// Parse: "pkg/path/file.go:startLine.startCol,endLine.endCol numStmt count"
		colonIdx := strings.LastIndex(line, ":")
		if colonIdx < 0 {
			continue
		}
		pkgFile := line[:colonIdx]

		rest := line[colonIdx+1:]
		parts := strings.Fields(rest)
		if len(parts) < 2 {
			continue
		}

		// Parse range: "startLine.startCol,endLine.endCol"
		rangeParts := strings.Split(parts[0], ",")
		if len(rangeParts) != 2 {
			continue
		}

		count, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil || count == 0 {
			continue
		}

		startLine := parseLineFromPos(rangeParts[0])
		endLine := parseLineFromPos(rangeParts[1])
		if startLine <= 0 || endLine <= 0 {
			continue
		}

		// Resolve to absolute path
		absFile := resolvePackageFile(pkgFile, moduleDir)

		for l := startLine; l <= endLine; l++ {
			key := fmt.Sprintf("%s:%d", absFile, l)
			cm.covered[key] = true
		}
	}

	return cm, scanner.Err()
}

// parseLineFromPos extracts the line number from "line.col" format.
func parseLineFromPos(s string) int {
	dotIdx := strings.Index(s, ".")
	if dotIdx < 0 {
		return 0
	}
	n, err := strconv.Atoi(s[:dotIdx])
	if err != nil {
		return 0
	}
	return n
}

// resolvePackageFile resolves a package-relative file path to an absolute path.
func resolvePackageFile(pkgFile string, moduleDir string) string {
	// pkgFile format is "module/path/file.go" — we need to find it relative to moduleDir.
	// Try direct resolution first.
	candidate := filepath.Join(moduleDir, pkgFile)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	// The pkgFile might include the module prefix. Try stripping it.
	// Find go.mod to get the module path.
	modPath := readModulePath(moduleDir)
	if modPath != "" && strings.HasPrefix(pkgFile, modPath) {
		relPath := strings.TrimPrefix(pkgFile, modPath)
		relPath = strings.TrimPrefix(relPath, "/")
		candidate = filepath.Join(moduleDir, relPath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return pkgFile
}

// readModulePath reads the module path from go.mod in the given directory.
func readModulePath(dir string) string {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

// IsCovered returns true if the given file:line is covered by any test.
func (cm *CoverageMap) IsCovered(file string, line int) bool {
	if cm == nil {
		return true // If no coverage data, assume covered (don't skip)
	}
	key := fmt.Sprintf("%s:%d", file, line)
	return cm.covered[key]
}

// TestsForLine returns the test function names that cover the given line.
// Currently returns nil (per-test coverage is a future optimization).
func (cm *CoverageMap) TestsForLine(file string, line int) []string {
	if cm == nil {
		return nil
	}
	key := fmt.Sprintf("%s:%d", file, line)
	return cm.lineTests[key]
}
