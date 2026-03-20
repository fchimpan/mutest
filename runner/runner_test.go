package runner

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fchimpan/mutest/engine"
	"github.com/fchimpan/mutest/mutator"
)

func TestRun_Integration(t *testing.T) {
	testDir, err := filepath.Abs(filepath.Join("..", "testdata", "project"))
	if err != nil {
		t.Fatal(err)
	}

	compMut := &mutator.ComparisonMutator{}
	eng := engine.New(testDir, compMut)

	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(points) == 0 {
		t.Fatal("no mutation points found")
	}

	cfg := Config{
		Workers: 2,
		Timeout: 30 * time.Second,
		BaseDir: testDir,
	}

	summary := Run(context.Background(), eng, compMut, points, cfg, nil)

	if summary.Total != len(points) {
		t.Errorf("expected Total=%d, got %d", len(points), summary.Total)
	}
	if summary.Killed+summary.Survived+summary.Errors != summary.Total {
		t.Errorf("killed(%d)+survived(%d)+errors(%d) != total(%d)",
			summary.Killed, summary.Survived, summary.Errors, summary.Total)
	}
	// IsPositive's > 0 -> >= 0 should be killed by TestIsPositive(0)
	if summary.Killed == 0 {
		t.Error("expected at least 1 killed mutant")
	}
	// Clamp boundaries are not fully tested, so some should survive
	if summary.Survived == 0 {
		t.Error("expected at least 1 survived mutant")
	}
	if summary.Errors != 0 {
		t.Errorf("expected 0 errors, got %d", summary.Errors)
	}
	if summary.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestRun_ProgressCallback(t *testing.T) {
	testDir, err := filepath.Abs(filepath.Join("..", "testdata", "project"))
	if err != nil {
		t.Fatal(err)
	}

	compMut := &mutator.ComparisonMutator{}
	eng := engine.New(testDir, compMut)

	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Workers: 1,
		Timeout: 30 * time.Second,
		BaseDir: testDir,
	}

	var callCount atomic.Int32
	progress := func(r Result, done, total int) {
		callCount.Add(1)
		if total != len(points) {
			t.Errorf("progress total=%d, want %d", total, len(points))
		}
		if done < 1 || done > total {
			t.Errorf("progress done=%d out of range [1, %d]", done, total)
		}
	}

	Run(context.Background(), eng, compMut, points, cfg, progress)

	if int(callCount.Load()) != len(points) {
		t.Errorf("progress called %d times, expected %d", callCount.Load(), len(points))
	}
}

func TestRun_EmptyPoints(t *testing.T) {
	eng := engine.New(".", &mutator.ComparisonMutator{})
	cfg := Config{Workers: 1, Timeout: 10 * time.Second, BaseDir: "."}

	summary := Run(context.Background(), eng, &mutator.ComparisonMutator{}, nil, cfg, nil)

	if summary.Total != 0 {
		t.Errorf("expected 0 total, got %d", summary.Total)
	}
	if summary.Killed != 0 || summary.Survived != 0 || summary.Errors != 0 {
		t.Error("expected all zero counts for empty input")
	}
}

func TestRun_PrepareError(t *testing.T) {
	testDir, err := filepath.Abs(filepath.Join("..", "testdata", "project"))
	if err != nil {
		t.Fatal(err)
	}

	compMut := &mutator.ComparisonMutator{}
	eng := engine.New(testDir, compMut)

	// Create a point with a nonexistent file to trigger Prepare error
	bogusPoints := []mutator.MutationPoint{
		{
			File:     "/nonexistent/file.go",
			Package:  "fake",
			Line:     1,
			Column:   1,
			NodeID:   0,
			Original: 15, // token.GTR
			Mutated:  16, // token.GEQ
			Desc:     "> to >=",
		},
	}

	cfg := Config{
		Workers: 1,
		Timeout: 10 * time.Second,
		BaseDir: testDir,
	}

	summary := Run(context.Background(), eng, compMut, bogusPoints, cfg, nil)

	if summary.Total != 1 {
		t.Errorf("expected Total=1, got %d", summary.Total)
	}
	if summary.Errors != 1 {
		t.Errorf("expected 1 error, got %d", summary.Errors)
	}
	if summary.Killed != 0 || summary.Survived != 0 {
		t.Error("expected 0 killed and 0 survived for error case")
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	testDir, err := filepath.Abs(filepath.Join("..", "testdata", "project"))
	if err != nil {
		t.Fatal(err)
	}

	compMut := &mutator.ComparisonMutator{}
	eng := engine.New(testDir, compMut)

	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cfg := Config{
		Workers: 1,
		Timeout: 30 * time.Second,
		BaseDir: testDir,
	}

	summary := Run(ctx, eng, compMut, points, cfg, nil)

	// All should be killed (context cancelled → tests fail)
	if summary.Total != len(points) {
		t.Errorf("expected Total=%d, got %d", len(points), summary.Total)
	}
}
