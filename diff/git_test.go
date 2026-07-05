package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseDiffOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		root  string
		want  ChangedLines
	}{
		{
			name: "single file single hunk",
			input: `diff --git a/calc.go b/calc.go
index abc1234..def5678 100644
--- a/calc.go
+++ b/calc.go
@@ -5,2 +5,3 @@ func Add(a, b int) int {
+	if a > b {
+		return a
+	}`,
			root: "/repo",
			want: ChangedLines{
				"/repo/calc.go": {5: true, 6: true, 7: true},
			},
		},
		{
			name: "multiple files",
			input: `diff --git a/a.go b/a.go
index 1111111..2222222 100644
--- a/a.go
+++ b/a.go
@@ -10,0 +11,2 @@ func Foo() {
+	x := 1
+	y := 2
diff --git a/b.go b/b.go
index 3333333..4444444 100644
--- a/b.go
+++ b/b.go
@@ -3,1 +3,1 @@ package main
-var old = 1
+var new = 2`,
			root: "/repo",
			want: ChangedLines{
				"/repo/a.go": {11: true, 12: true},
				"/repo/b.go": {3: true},
			},
		},
		{
			name: "new file (all lines added)",
			input: `diff --git a/new.go b/new.go
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package main
+
+func New() {}`,
			root: "/repo",
			want: ChangedLines{
				"/repo/new.go": {1: true, 2: true, 3: true},
			},
		},
		{
			name: "deleted file (no added lines)",
			input: `diff --git a/old.go b/old.go
deleted file mode 100644
index 1234567..0000000
--- a/old.go
+++ /dev/null
@@ -1,5 +0,0 @@
-package main
-
-func Old() {}
-
-// end`,
			root: "/repo",
			want: ChangedLines{},
		},
		{
			name: "multiple hunks in one file",
			input: `diff --git a/multi.go b/multi.go
index aaa..bbb 100644
--- a/multi.go
+++ b/multi.go
@@ -5,1 +5,1 @@ func A() {
-	return 1
+	return 2
@@ -20,0 +20,3 @@ func B() {
+	x := 1
+	y := 2
+	z := 3`,
			root: "/repo",
			want: ChangedLines{
				"/repo/multi.go": {5: true, 20: true, 21: true, 22: true},
			},
		},
		{
			name: "single line hunk (count omitted)",
			input: `diff --git a/single.go b/single.go
index aaa..bbb 100644
--- a/single.go
+++ b/single.go
@@ -7 +7 @@ func F() {
-	old
+	new`,
			root: "/repo",
			want: ChangedLines{
				"/repo/single.go": {7: true},
			},
		},
		{
			name:  "empty diff output",
			input: "",
			root:  "/repo",
			want:  ChangedLines{},
		},
		{
			name: "file in directory containing b/ in path",
			input: `diff --git a/pkg/b/foo.go b/pkg/b/foo.go
index aaa..bbb 100644
--- a/pkg/b/foo.go
+++ b/pkg/b/foo.go
@@ -1,1 +1,1 @@ package b
-var x = 1
+var x = 2`,
			root: "/repo",
			want: ChangedLines{
				"/repo/pkg/b/foo.go": {1: true},
			},
		},
		{
			name: "renamed file uses new name",
			input: `diff --git a/old_name.go b/new_name.go
similarity index 90%
rename from old_name.go
rename to new_name.go
index aaa..bbb 100644
--- a/old_name.go
+++ b/new_name.go
@@ -3,1 +3,1 @@ package main
-var x = 1
+var x = 2`,
			root: "/repo",
			want: ChangedLines{
				"/repo/new_name.go": {3: true},
			},
		},
		{
			// A "+++ /dev/null" header must reset the current file so that any
			// following hunk lines are not attributed to a stale path. Real git
			// deletions carry "+0,0" hunks (no added lines), so this feeds an
			// intentionally malformed hunk to pin the reset behavior.
			name: "dev-null header ignores following added lines",
			input: `diff --git a/gone.go b/gone.go
deleted file mode 100644
index 1234567..0000000
--- a/gone.go
+++ /dev/null
@@ -1,2 +1,2 @@
+ghost
+lines`,
			root: "/repo",
			want: ChangedLines{},
		},
		{
			// The file is identified from the "+++ b/<path>" header, which stays
			// unambiguous even when a directory name contains " b/". The legacy
			// "diff --git a/x b/y" parsing (LastIndex " b/") would mis-split here.
			name: "directory name containing ' b/' segment",
			input: `diff --git a/a b/lib.go b/a b/lib.go
index aaa..bbb 100644
--- a/a b/lib.go
+++ b/a b/lib.go
@@ -1,1 +1,1 @@ package a
-var x = 1
+var x = 2`,
			root: "/repo",
			want: ChangedLines{
				"/repo/a b/lib.go": {1: true},
			},
		},
		{
			// With `-c core.quotepath=off` git emits non-ASCII bytes verbatim, so
			// the +++ header carries the real path.
			name: "non-ASCII path emitted verbatim",
			input: `diff --git a/計算.go b/計算.go
index aaa..bbb 100644
--- a/計算.go
+++ b/計算.go
@@ -3,1 +3,1 @@ package calc
-var x = 1
+var x = 2`,
			root: "/repo",
			want: ChangedLines{
				"/repo/計算.go": {3: true},
			},
		},
		{
			// git appends a trailing TAB to ---/+++ header paths that contain
			// spaces (GNU diff convention); the parser must strip it.
			name: "path with space has trailing tab in header",
			input: "diff --git a/has space.go b/has space.go\n" +
				"index aaa..bbb 100644\n" +
				"--- a/has space.go\t\n" +
				"+++ b/has space.go\t\n" +
				"@@ -3,1 +3,1 @@ package main\n" +
				"-var x = 1\n" +
				"+var x = 2",
			root: "/repo",
			want: ChangedLines{
				"/repo/has space.go": {3: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDiffOutput([]byte(tt.input), tt.root)

			if len(got) != len(tt.want) {
				t.Fatalf("got %d files, want %d\ngot: %v", len(got), len(tt.want), got)
			}

			for file, wantLines := range tt.want {
				gotLines, ok := got[file]
				if !ok {
					t.Errorf("missing file %s", file)
					continue
				}
				if len(gotLines) != len(wantLines) {
					t.Errorf("file %s: got %d lines, want %d\ngot: %v\nwant: %v", file, len(gotLines), len(wantLines), gotLines, wantLines)
					continue
				}
				for line := range wantLines {
					if !gotLines[line] {
						t.Errorf("file %s: missing line %d", file, line)
					}
				}
			}
		})
	}
}

// runGit runs a git command inside dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// writeRepoFile writes name (which may contain slashes) under dir.
func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupRepo creates a throwaway git repo, writes initial, commits it, and marks
// that base commit with the "base" branch so tests can diff against "base". It
// returns the working-tree root (symlinks resolved so it matches the path that
// ParseGitDiff derives from `git rev-parse --show-toplevel`).
func setupRepo(t *testing.T, initial map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "mutest test")
	// Host config may enable commit signing (e.g. SSH signing via 1Password);
	// disable it so the test's commits succeed hermetically.
	runGit(t, dir, "config", "commit.gpgsign", "false")
	runGit(t, dir, "config", "tag.gpgsign", "false")

	for name, content := range initial {
		writeRepoFile(t, dir, name, content)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	runGit(t, dir, "branch", "base")
	return dir
}

// chdirRepo switches the process working directory to dir for the test's
// duration. ParseGitDiff shells out to git in the current directory, so tests
// that use it must not run in parallel.
func chdirRepo(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

func TestParseGitDiff_Integration(t *testing.T) {
	const baseLib = "package pkg\n\nfunc Old(n int) bool { return n > 100 }\n"

	tests := []struct {
		name    string
		initial map[string]string
		// mutate applies post-base changes (working-tree edits and/or commits).
		mutate func(t *testing.T, dir string)
		// wantLines requires each listed line to be reported for the rel path.
		wantLines map[string][]int
		// wantWhole requires the rel path to be present with a nil (whole-file)
		// line set, the convention used for untracked files.
		wantWhole []string
		// absentLines requires each listed line to be absent for the rel path.
		absentLines map[string][]int
	}{
		{
			name:    "committed change is detected (regression)",
			initial: map[string]string{"pkg/lib.go": baseLib},
			mutate: func(t *testing.T, dir string) {
				writeRepoFile(t, dir, "pkg/lib.go",
					baseLib+"\nfunc New(n int) bool { return n < 5 }\n")
				runGit(t, dir, "add", "-A")
				runGit(t, dir, "commit", "-q", "-m", "add New")
			},
			wantLines: map[string][]int{"pkg/lib.go": {5}},
		},
		{
			name:    "tracked uncommitted change is detected",
			initial: map[string]string{"pkg/lib.go": baseLib},
			mutate: func(t *testing.T, dir string) {
				writeRepoFile(t, dir, "pkg/lib.go",
					baseLib+"\nfunc New(n int) bool { return n < 5 }\n")
			},
			wantLines: map[string][]int{"pkg/lib.go": {5}},
		},
		{
			name:    "untracked new file counts as a whole-file change",
			initial: map[string]string{"pkg/lib.go": baseLib},
			mutate: func(t *testing.T, dir string) {
				writeRepoFile(t, dir, "pkg/extra.go",
					"package pkg\n\nfunc Extra(n int) bool { return n <= 9 }\n")
			},
			wantWhole: []string{"pkg/extra.go"},
		},
		{
			name:    "uncommitted line shift uses working-tree line numbers",
			initial: map[string]string{"pkg/lib.go": baseLib},
			mutate: func(t *testing.T, dir string) {
				// Insert a new function above Old so Old moves down without its
				// content changing. The new comparison must be reported at its
				// working-tree line (3), and Old's new line (5) must not be.
				writeRepoFile(t, dir, "pkg/lib.go",
					"package pkg\n\nfunc New(n int) bool { return n < 5 }\n\nfunc Old(n int) bool { return n > 100 }\n")
			},
			wantLines:   map[string][]int{"pkg/lib.go": {3}},
			absentLines: map[string][]int{"pkg/lib.go": {5}},
		},
		{
			name:    "committed non-ASCII filename is detected",
			initial: map[string]string{"pkg/lib.go": baseLib},
			mutate: func(t *testing.T, dir string) {
				writeRepoFile(t, dir, "pkg/計算.go",
					"package pkg\n\nfunc JP(n int) bool { return n >= 7 }\n")
				runGit(t, dir, "add", "-A")
				runGit(t, dir, "commit", "-q", "-m", "add non-ascii file")
			},
			wantLines: map[string][]int{"pkg/計算.go": {3}},
		},
		{
			name:    "untracked filename with a space is detected",
			initial: map[string]string{"pkg/lib.go": baseLib},
			mutate: func(t *testing.T, dir string) {
				writeRepoFile(t, dir, "pkg/has space.go",
					"package pkg\n\nfunc SP(n int) bool { return n <= 9 }\n")
			},
			wantWhole: []string{"pkg/has space.go"},
		},
		{
			// A tracked path with a space exercises the trailing-tab header
			// convention end-to-end (see the unit test above for the raw form).
			name: "tracked uncommitted change in filename with a space",
			initial: map[string]string{
				"pkg/lib.go":       baseLib,
				"pkg/has space.go": "package pkg\n\nfunc SP(n int) bool { return n <= 9 }\n",
			},
			mutate: func(t *testing.T, dir string) {
				writeRepoFile(t, dir, "pkg/has space.go",
					"package pkg\n\nfunc SP(n int) bool { return n <= 10 }\n")
			},
			wantLines: map[string][]int{"pkg/has space.go": {3}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupRepo(t, tt.initial)
			if tt.mutate != nil {
				tt.mutate(t, dir)
			}
			chdirRepo(t, dir)

			cl, err := ParseGitDiff("base")
			if err != nil {
				t.Fatalf("ParseGitDiff: %v", err)
			}

			checkChangedLines(t, dir, cl, tt.wantLines, tt.wantWhole, tt.absentLines)
		})
	}
}

// checkChangedLines asserts cl against per-file expectations expressed as
// paths relative to root.
func checkChangedLines(t *testing.T, root string, cl ChangedLines, wantLines map[string][]int, wantWhole []string, absentLines map[string][]int) {
	t.Helper()

	for rel, lines := range wantLines {
		abs := filepath.Join(root, rel)
		got, ok := cl[abs]
		if !ok {
			t.Fatalf("expected %s to be reported as changed; got %v", rel, cl)
		}
		for _, ln := range lines {
			if !got[ln] {
				t.Errorf("expected %s line %d to be changed; got %v", rel, ln, got)
			}
		}
	}
	for _, rel := range wantWhole {
		abs := filepath.Join(root, rel)
		lines, ok := cl[abs]
		if !ok {
			t.Fatalf("expected %s to be reported; got %v", rel, cl)
		}
		if lines != nil {
			t.Errorf("expected %s to have a nil (whole-file) line set; got %v", rel, lines)
		}
	}
	for rel, lines := range absentLines {
		abs := filepath.Join(root, rel)
		got := cl[abs]
		for _, ln := range lines {
			if got[ln] {
				t.Errorf("expected %s line %d to be unchanged, but it was reported", rel, ln)
			}
		}
	}
}

func TestParseGitDiff_SymlinkedWorkingDirectory(t *testing.T) {
	// On macOS the default TMPDIR sits behind a symlink (/var -> /private/var),
	// so a process's logical working directory (what $PWD and os.Getwd report,
	// and what the Go toolchain uses for package file paths) can differ from
	// the physical path git reports. ChangedLines keys must match the logical
	// view, or FilterPoints silently drops every point.
	const lib = "package pkg\n\nfunc Old(n int) bool { return n > 100 }\n"
	repo := setupRepo(t, map[string]string{"pkg/lib.go": lib})
	writeRepoFile(t, repo, "pkg/lib.go", lib+"\nfunc New(n int) bool { return n < 5 }\n")

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	chdirRepo(t, link)
	// os.Getwd prefers $PWD when it names the current directory, which is how
	// a shell cd through a symlink presents a logical path to subprocesses.
	t.Setenv("PWD", link)

	cl, err := ParseGitDiff("base")
	if err != nil {
		t.Fatalf("ParseGitDiff: %v", err)
	}

	checkChangedLines(t, link, cl, map[string][]int{"pkg/lib.go": {5}}, nil, nil)
}
