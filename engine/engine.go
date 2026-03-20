package engine

import (
	"bytes"
	"encoding/json"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/fchimpan/mutest/mutator"
)

// Overlay represents the Go overlay JSON structure used by -overlay flag.
type Overlay struct {
	Replace map[string]string `json:"Replace"`
}

// Mutant is a fully prepared mutation ready for testing.
type Mutant struct {
	Point       mutator.MutationPoint
	OverlayPath string // path to the overlay.json temp file
	TempDir     string // directory containing overlay.json and mutated file
}

// Engine scans packages, discovers mutations, and prepares overlays.
type Engine struct {
	mutators []mutator.Mutator
	baseDir  string
}

// New creates an Engine rooted at baseDir with the given mutators.
func New(baseDir string, mutators ...mutator.Mutator) *Engine {
	return &Engine{
		mutators: mutators,
		baseDir:  baseDir,
	}
}

// DiscoverAll parses all non-test .go files under baseDir and returns
// all mutation points found by the registered mutators.
func (e *Engine) DiscoverAll() ([]mutator.MutationPoint, error) {
	absBase, err := filepath.Abs(e.baseDir)
	if err != nil {
		return nil, err
	}

	var points []mutator.MutationPoint

	err = filepath.WalkDir(absBase, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		name := d.Name()

		if d.IsDir() {
			if name == "vendor" || name == "testdata" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil // skip unparseable files
		}

		pkg := file.Name.Name

		for _, m := range e.mutators {
			pts := m.Discover(fset, file, path, pkg)
			points = append(points, pts...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return points, nil
}

// Prepare re-parses the source file for the given mutation point,
// applies the mutation, writes a temp file, and generates an overlay.json.
func (e *Engine) Prepare(m mutator.Mutator, point mutator.MutationPoint) (_ *Mutant, retErr error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, point.File, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	m.Apply(file, point)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "mutest-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(tempDir)
		}
	}()

	mutatedPath := filepath.Join(tempDir, "mutated.go")
	if err := os.WriteFile(mutatedPath, buf.Bytes(), 0644); err != nil {
		return nil, err
	}

	overlay := Overlay{
		Replace: map[string]string{
			point.File: mutatedPath,
		},
	}
	overlayData, err := json.Marshal(overlay)
	if err != nil {
		return nil, err
	}

	overlayPath := filepath.Join(tempDir, "overlay.json")
	if err := os.WriteFile(overlayPath, overlayData, 0644); err != nil {
		return nil, err
	}

	return &Mutant{
		Point:       point,
		OverlayPath: overlayPath,
		TempDir:     tempDir,
	}, nil
}

// Cleanup removes the temp directory for a mutant.
func (e *Engine) Cleanup(m *Mutant) {
	os.RemoveAll(m.TempDir)
}
