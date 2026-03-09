package cli

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/fchimpan/mutest/internal/mutator"
	"github.com/fchimpan/mutest/internal/pipeline"
	"github.com/fchimpan/mutest/internal/report"
)

func runMutest(cmd *cobra.Command, args []string) error {
	patterns := args
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	flags := cmd.Flags()

	workers, _ := flags.GetInt("workers")
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	timeout, _ := flags.GetDuration("timeout")
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	mutatorNames, _ := flags.GetStringSlice("mutators")
	noSSA, _ := flags.GetBool("no-ssa")
	noCov, _ := flags.GetBool("no-coverage")
	verbose, _ := flags.GetBool("verbose")
	jsonOutput, _ := flags.GetBool("json")
	dryRun, _ := flags.GetBool("dry-run")
	minScore, _ := flags.GetFloat64("min-score")

	var reporter report.Reporter
	if jsonOutput {
		reporter = report.NewJSONReporter(os.Stdout, verbose)
	} else {
		reporter = report.NewConsoleReporter(os.Stdout, verbose)
	}

	cfg := pipeline.Config{
		Dir:       dir,
		Patterns:  patterns,
		Mutators:  mutator.SelectMutators(mutatorNames),
		Workers:   workers,
		Timeout:   timeout,
		EnableSSA: !noSSA,
		EnableCov: !noCov,
		DryRun:    dryRun,
	}

	p := pipeline.New(cfg, reporter)
	summary, err := p.Execute(cmd.Context())
	if err != nil {
		return err
	}

	if minScore > 0 && summary.MutationScore < minScore {
		return fmt.Errorf("mutation score %.1f%% is below minimum %.1f%%", summary.MutationScore, minScore)
	}

	return nil
}
