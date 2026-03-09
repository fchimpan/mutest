package analysis

import (
	"errors"
	"fmt"

	"golang.org/x/tools/go/packages"
)

// LoadConfig holds configuration for package loading.
type LoadConfig struct {
	Dir      string   // Working directory
	Patterns []string // Package patterns (e.g., "./...")
}

// LoadPackages loads Go packages with syntax and type information.
func LoadPackages(cfg LoadConfig) ([]*packages.Package, error) {
	loadCfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo |
			packages.NeedTypesSizes,
		Dir:   cfg.Dir,
		Tests: false,
	}
	pkgs, err := packages.Load(loadCfg, cfg.Patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}
	var errs []error
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, e := range pkg.Errors {
			errs = append(errs, e)
		}
	})
	if len(errs) > 0 {
		return nil, fmt.Errorf("package errors: %w", errors.Join(errs...))
	}
	return pkgs, nil
}
