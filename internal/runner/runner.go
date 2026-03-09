package runner

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/fchimpan/mutest/internal/mutator"
)

// Result holds the outcome of running tests against a single mutant.
type Result struct {
	Mutation mutator.Mutation `json:"mutation"`
	Status   mutator.MutantStatus `json:"status"`
	Output   string               `json:"output,omitempty"`
	Duration time.Duration        `json:"duration"`
}

// RunConfig holds configuration for test execution.
type RunConfig struct {
	Dir      string        // Working directory (module root)
	Packages []string      // Packages to test (e.g., ["./..."])
	Timeout  time.Duration // Per-mutant test timeout
}

// Runner executes tests against mutants using Go's overlay mechanism.
type Runner struct {
	config  RunConfig
	overlay *OverlayManager
}

func NewRunner(config RunConfig) (*Runner, error) {
	om, err := NewOverlayManager()
	if err != nil {
		return nil, err
	}
	return &Runner{config: config, overlay: om}, nil
}

// Run executes tests against a single mutant. coveringTests specifies which
// test functions to run; if empty, all tests are run.
func (r *Runner) Run(ctx context.Context, m mutator.Mutation, coveringTests []string) Result {
	if m.MutatedSource == nil {
		return Result{Mutation: m, Status: mutator.StatusBuildError, Output: "no mutated source"}
	}

	overlayPath, cleanup, err := r.overlay.CreateOverlay(m.File, m.MutatedSource)
	if err != nil {
		return Result{Mutation: m, Status: mutator.StatusBuildError, Output: err.Error()}
	}
	defer cleanup()

	// Phase 1: Compile check
	if err := r.buildCheck(ctx, overlayPath); err != nil {
		return Result{Mutation: m, Status: mutator.StatusBuildError, Output: err.Error()}
	}

	// Phase 2: Test execution
	ctx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	start := time.Now()
	output, testErr := r.runTests(ctx, overlayPath, coveringTests)
	duration := time.Since(start)

	result := Result{
		Mutation: m,
		Output:   output,
		Duration: duration,
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.Status = mutator.StatusTimeout
	} else if testErr != nil {
		result.Status = mutator.StatusKilled
	} else {
		result.Status = mutator.StatusSurvived
	}

	return result
}

func (r *Runner) buildCheck(ctx context.Context, overlayPath string) error {
	args := []string{"build", "-overlay", overlayPath}
	args = append(args, r.config.Packages...)

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = r.config.Dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed: %s", string(output))
	}
	return nil
}

func (r *Runner) runTests(ctx context.Context, overlayPath string, coveringTests []string) (string, error) {
	args := []string{"test", "-overlay", overlayPath, "-count=1", "-failfast"}

	if len(coveringTests) > 0 {
		pattern := "^(" + strings.Join(coveringTests, "|") + ")$"
		args = append(args, "-run", pattern)
	}

	args = append(args, r.config.Packages...)

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = r.config.Dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// Close cleans up temporary files.
func (r *Runner) Close() error {
	return r.overlay.Close()
}
