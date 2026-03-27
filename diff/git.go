package diff

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var hunkRe = regexp.MustCompile(`@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// ParseGitDiff runs git diff against baseRef and returns the changed lines
// in Go source files. It uses the three-dot syntax (base...HEAD) to diff
// from the merge-base, which is the correct behavior for PR branches.
//
//mutest:skip
func ParseGitDiff(baseRef string) (ChangedLines, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, fmt.Errorf("git repo root: %w", err)
	}

	out, err := exec.Command(
		"git", "diff", "--unified=0", "--no-color",
		baseRef+"...HEAD", "--", "*.go",
	).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git diff failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("git diff: %w", err)
	}

	return parseDiffOutput(out, root), nil
}

// repoRoot returns the absolute path of the git repository root.
func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

// parseDiffOutput parses unified diff output (with --unified=0)
// and returns ChangedLines with absolute file paths.
func parseDiffOutput(output []byte, root string) ChangedLines {
	cl := make(ChangedLines)
	var currentFile string

	for line := range strings.SplitSeq(string(output), "\n") {
		if strings.HasPrefix(line, "diff --git") {
			// Extract the b/ path: "diff --git a/foo.go b/foo.go"
			// Use LastIndex to handle paths containing " b/" as a directory component.
			if idx := strings.LastIndex(line, " b/"); idx >= 0 { //mutest:skip
				currentFile = filepath.Join(root, line[idx+3:])
			}
			continue
		}

		if !strings.HasPrefix(line, "@@") || currentFile == "" {
			continue
		}

		matches := hunkRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		start, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}

		count := 1
		if matches[2] != "" {
			count, err = strconv.Atoi(matches[2])
			if err != nil {
				continue
			}
		}

		// count == 0 means pure deletion at this position; no new lines to track.
		if count == 0 {
			continue
		}

		if cl[currentFile] == nil {
			cl[currentFile] = make(map[int]bool)
		}
		for i := start; i < start+count; i++ {
			cl[currentFile][i] = true
		}
	}

	return cl
}
