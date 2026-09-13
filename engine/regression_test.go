package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fchimpan/mutest/mutator"
)

func TestDiscoverDependencyConstants(t *testing.T) {
	for _, mode := range []string{"replacement", "gopath vendor"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("GOWORK", "off")
			var dir string
			files := map[string]string{}
			if mode == "replacement" {
				t.Setenv("GO111MODULE", "on")
				dir = filepath.Join(root, "app")
				files["app/go.mod"] = "module example.org/app\ngo 1.24.0\nrequire example.org/dep v0.0.0\nreplace example.org/dep => ../dep\n"
				files["dep/go.mod"] = "module example.org/dep\ngo 1.24.0\n"
				files["dep/dep.go"] = "package dep\nconst Huge = 1 << 100\n"
			} else {
				t.Setenv("GO111MODULE", "off")
				t.Setenv("GOPATH", root)
				dir = filepath.Join(root, "src/example.org/app")
				files["src/example.org/app/vendor/example.org/dep/dep.go"] = "package dep\nconst Huge = 1 << 100\n"
			}
			for path, src := range files {
				path = filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(src), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package app\nimport \"example.org/dep\"\nfunc F() bool {return dep.Huge > 0}\n"), 0644); err != nil {
				t.Fatal(err)
			}
			chdir(t, dir)
			e := New([]string{"."}, &mutator.ComparisonMutator{})
			points, err := e.DiscoverAll()
			if err != nil {
				t.Fatal(err)
			}
			if len(points) != 1 || !points[0].Constant {
				t.Fatalf("constant metadata lost: %+v", points)
			}
		})
	}
}

func TestDiscoverRelativeImports(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GO111MODULE", "off")
	t.Setenv("GOWORK", "off")
	if err := os.Mkdir(filepath.Join(dir, "dep"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{
		"lib.go": `package p
import "./dep"
func F() bool {return dep.Huge > 0}
`,
		"dep/dep.go": "package dep\nconst Huge=1<<100\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)
	points, err := New([]string{"./..."}, &mutator.ComparisonMutator{}).DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || !points[0].Constant {
		t.Fatalf("wrong discovery: %+v", points)
	}
}

func TestDiscoverUsesGoEnvArchitecture(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"go.mod": "module p\ngo 1.24.0\n",
		"goenv":  "GOARCH=386\nGOOS=linux\n",
		"lib.go": `package p
import "unsafe"
func F(x int) int {if x<int(unsafe.Sizeof(int(0))) {x=8};return x}
`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GOENV", filepath.Join(dir, "goenv"))
	t.Setenv("GOARCH", "")
	t.Setenv("GOOS", "")
	t.Setenv("GOWORK", "off")
	chdir(t, dir)
	points, err := New([]string{"."}, &mutator.ComparisonMutator{}).DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	// On the selected 32-bit target this compares x to 4 and assigns 8.
	// It is not an equivalent clamp, even when mutest itself runs on 64-bit.
	if len(points) != 1 {
		t.Fatalf("target architecture ignored: %+v", points)
	}
}
