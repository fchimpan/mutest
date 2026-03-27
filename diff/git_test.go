package diff

import (
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
			name: "empty diff output",
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
