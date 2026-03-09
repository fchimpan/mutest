// Package cli provides the command-line interface for mutest.
package cli

import (
	"github.com/spf13/cobra"
)

var version = "dev"

// NewRootCmd creates the root command. Usage follows go test conventions:
//
//	mutest ./...
//	mutest ./pkg/...
func NewRootCmd(v string) *cobra.Command {
	version = v
	root := &cobra.Command{
		Use:   "mutest [packages]",
		Short: "Mutation testing for Go with SSA-based equivalent mutant detection",
		Long: `mutest runs mutation testing on Go packages, similar to how go test runs tests.

Examples:
  mutest ./...              # Test all packages
  mutest ./pkg/foo          # Test a specific package
  mutest -v ./...           # Verbose output
  mutest -json ./...        # JSON output
  mutest -dry-run ./...     # List mutations without running tests
  mutest -workers 8 ./...   # Use 8 parallel workers`,
		Args:                  cobra.ArbitraryArgs,
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
		RunE:                  runMutest,
	}
	root.TraverseChildren = true

	f := root.Flags()
	f.IntP("workers", "w", 0, "number of parallel workers (default: NumCPU)")
	f.DurationP("timeout", "t", 0, "timeout per mutant test (default: 30s)")
	f.StringSliceP("mutators", "m", nil, "mutators to enable (default: all)")
	f.Bool("no-ssa", false, "disable SSA-based equivalent mutant detection")
	f.Bool("no-coverage", false, "disable coverage-based filtering")
	f.BoolP("verbose", "v", false, "verbose output showing each mutation result")
	f.Bool("json", false, "output results as JSON")
	f.Bool("dry-run", false, "list mutations without running tests")
	f.Float64("min-score", 0, "minimum mutation score (0-100), exit 1 if below")

	root.AddCommand(newVersionCmd())
	return root
}
