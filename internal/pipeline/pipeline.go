// Package pipeline orchestrates the full mutation testing process.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fchimpan/mutest/internal/analysis"
	"github.com/fchimpan/mutest/internal/mutation"
	"github.com/fchimpan/mutest/internal/mutator"
	"github.com/fchimpan/mutest/internal/report"
	"github.com/fchimpan/mutest/internal/runner"
)

// Config holds all pipeline configuration.
type Config struct {
	Dir        string
	Patterns   []string
	Mutators   []mutator.Mutator
	Workers    int
	Timeout    time.Duration
	EnableSSA  bool
	EnableCov  bool
	DryRun     bool
}

// Pipeline orchestrates the full mutation testing process.
type Pipeline struct {
	config    Config
	generator *mutation.Generator
	applier   *mutation.Applier
	reporter  report.Reporter
}

func New(cfg Config, reporter report.Reporter) *Pipeline {
	return &Pipeline{
		config:    cfg,
		generator: mutation.NewGenerator(cfg.Mutators),
		applier:   &mutation.Applier{},
		reporter:  reporter,
	}
}

// Execute runs the full mutation testing pipeline. Returns a summary and a non-nil error on failure.
func (p *Pipeline) Execute(ctx context.Context) (*report.Summary, error) {
	start := time.Now()

	// ── Phase 1: Load packages ──
	pkgs, err := analysis.LoadPackages(analysis.LoadConfig{
		Dir:      p.config.Dir,
		Patterns: p.config.Patterns,
	})
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}

	// ── Phase 2: Generate mutations ──
	var allMutations []mutator.Mutation
	originalSources := make(map[string][]byte)

	for _, pkg := range pkgs {
		for i, file := range pkg.Syntax {
			filePath := pkg.CompiledGoFiles[i]
			source, err := os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", filePath, err)
			}
			originalSources[filePath] = source
			mutations := p.generator.Generate(pkg.Fset, filePath, file)
			allMutations = append(allMutations, mutations...)
		}
	}
	p.reporter.OnMutationsGenerated(len(allMutations))

	if len(allMutations) == 0 {
		return p.reporter.Summarize(nil, start), nil
	}

	// ── Phase 3: Coverage filtering ──
	var coverageMap *analysis.CoverageMap
	if p.config.EnableCov {
		cm, err := analysis.BuildCoverageMap(p.config.Dir, p.config.Patterns)
		if err != nil {
			// Coverage failure is not fatal; log and continue without filtering
			fmt.Fprintf(os.Stderr, "warning: coverage analysis failed: %v\n", err)
		} else {
			coverageMap = cm
		}
	}

	if coverageMap != nil {
		var covered []mutator.Mutation
		notCoveredCount := 0
		for i := range allMutations {
			if coverageMap.IsCovered(allMutations[i].File, allMutations[i].Line) {
				covered = append(covered, allMutations[i])
			} else {
				allMutations[i].Status = mutator.StatusNotCovered
				notCoveredCount++
			}
		}
		p.reporter.OnNotCoveredFiltered(notCoveredCount)
		allMutations = covered
	}

	// ── Phase 4: SSA equivalence filtering ──
	if p.config.EnableSSA {
		analyzer, err := analysis.NewEquivalenceAnalyzer(pkgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: SSA analysis failed: %v\n", err)
		} else {
			var filtered []mutator.Mutation
			equivalentCount := 0
			for _, m := range allMutations {
				if analyzer.IsEquivalent(m) {
					m.Status = mutator.StatusEquivalent
					equivalentCount++
				} else {
					filtered = append(filtered, m)
				}
			}
			p.reporter.OnEquivalentsFiltered(equivalentCount)
			allMutations = filtered
		}
	}

	// ── Phase 5: Apply mutations ──
	var applicable []mutator.Mutation
	for i := range allMutations {
		m := &allMutations[i]
		source, ok := originalSources[m.File]
		if !ok {
			continue
		}
		mutatedSource, err := p.applier.Apply(source, *m)
		if err != nil {
			continue
		}
		m.MutatedSource = mutatedSource
		applicable = append(applicable, *m)
	}

	// ── Dry run: skip test execution ──
	if p.config.DryRun {
		var results []runner.Result
		for _, m := range applicable {
			m.Status = mutator.StatusSkipped
			results = append(results, runner.Result{Mutation: m, Status: mutator.StatusSkipped})
		}
		return p.reporter.Summarize(results, start), nil
	}

	// ── Phase 6: Execute tests ──
	r, err := runner.NewRunner(runner.RunConfig{
		Dir:      p.config.Dir,
		Packages: p.config.Patterns,
		Timeout:  p.config.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("creating runner: %w", err)
	}
	defer r.Close()

	results := p.executeParallel(ctx, r, applicable, coverageMap)

	// ── Phase 7: Report ──
	return p.reporter.Summarize(results, start), nil
}

func (p *Pipeline) executeParallel(ctx context.Context, r *runner.Runner, mutations []mutator.Mutation, cm *analysis.CoverageMap) []runner.Result {
	resultsCh := make(chan runner.Result, len(mutations))
	sem := make(chan struct{}, p.config.Workers)
	var wg sync.WaitGroup

	for _, m := range mutations {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(m mutator.Mutation) {
			defer wg.Done()
			defer func() { <-sem }()

			var coveringTests []string
			if cm != nil {
				coveringTests = cm.TestsForLine(m.File, m.Line)
			}

			result := r.Run(ctx, m, coveringTests)
			result.Mutation.Status = result.Status
			resultsCh <- result
			p.reporter.OnMutantResult(result)
		}(m)
	}

	wg.Wait()
	close(resultsCh)

	var results []runner.Result
	for res := range resultsCh {
		results = append(results, res)
	}
	return results
}
