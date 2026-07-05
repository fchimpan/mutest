package diff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var hunkRe = regexp.MustCompile(`@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// ParseGitDiff returns the changed lines in Go source files relative to
// baseRef. It resolves the merge-base of baseRef and HEAD and diffs it
// against the working tree, so staged and unstaged edits are included and
// line numbers match the on-disk sources that mutation discovery parses.
// Untracked (but not ignored) .go files are reported as whole-file changes
// using the nil line set convention described on ChangedLines.
//
//mutest:skip
func ParseGitDiff(baseRef string) (ChangedLines, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, fmt.Errorf("git repo root: %w", err)
	}

	mbOut, err := exec.Command("git", "merge-base", baseRef, "HEAD").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git merge-base %s HEAD failed: %s", baseRef, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("git merge-base: %w", err)
	}
	mb := strings.TrimSpace(string(mbOut))

	// Diffing a single commit (no second revision) compares it against the
	// working tree, which is what mutation discovery parses. core.quotepath=off
	// makes git print non-ASCII paths verbatim instead of quoting and escaping
	// them (see parseDiffOutput).
	out, err := exec.Command(
		"git", "-c", "core.quotepath=off", "diff", "--unified=0", "--no-color",
		mb, "--", "*.go",
	).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git diff failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("git diff: %w", err)
	}

	cl := parseDiffOutput(out, root)
	if err := addUntrackedFiles(cl, root); err != nil {
		return nil, err
	}
	return cl, nil
}

// addUntrackedFiles marks every untracked (but not ignored) Go file as a
// whole-file change in cl, using the nil line set convention described on
// ChangedLines. git diff does not report untracked files, so without this
// step brand-new files would silently escape diff mode.
func addUntrackedFiles(cl ChangedLines, root string) error {
	out, err := exec.Command(
		"git", "-c", "core.quotepath=off", "ls-files",
		"--others", "--exclude-standard", "--full-name", "--", "*.go",
	).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("git ls-files failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return fmt.Errorf("git ls-files: %w", err)
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if line == "" {
			continue
		}
		cl[filepath.Join(root, line)] = nil
	}
	return nil
}

// repoRoot returns the absolute path of the git repository root as seen from
// this process's working directory. It is derived from `git rev-parse
// --show-cdup` (a relative path) joined onto os.Getwd rather than taken from
// `--show-toplevel`, because the latter reports the physical path while
// os.Getwd and the Go toolchain (the source of MutationPoint file paths)
// report the logical, symlink-preserving one. The two views differ e.g. under
// the macOS temp dir (/var -> /private/var), and ChangedLines keys must match
// the Go toolchain's view for FilterPoints to work.
func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-cdup").Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	return filepath.Join(cwd, strings.TrimSpace(string(out))), nil
}

// parseDiffOutput parses unified diff output (with --unified=0) and returns
// ChangedLines with absolute file paths. Files are identified from the
// "+++ b/<path>" header, which names the post-image path even for renames;
// "+++ /dev/null" (a deletion) resets the current file so stray hunk lines
// are never attributed to a stale path.
//
// The output must come from git run with -c core.quotepath=off so that
// non-ASCII paths appear verbatim. Known limitation: paths containing double
// quotes or control characters are quoted by git even with quotepath=off and
// are not recognized here.
func parseDiffOutput(output []byte, root string) ChangedLines {
	cl := make(ChangedLines)
	var currentFile string

	for line := range strings.SplitSeq(string(output), "\n") {
		if p, ok := strings.CutPrefix(line, "+++ "); ok {
			// git appends a TAB after header paths that contain spaces
			// (GNU diff convention).
			p = strings.TrimSuffix(p, "\t")
			if p == "/dev/null" {
				currentFile = ""
			} else {
				currentFile = filepath.Join(root, strings.TrimPrefix(p, "b/"))
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
