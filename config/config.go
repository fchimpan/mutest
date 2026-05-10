// Package config defines the parsed CLI configuration for mutest.
package config

import (
	"fmt"
	"time"
)

// Config holds the parsed CLI flags that drive the mutation testing pipeline.
type Config struct {
	Patterns           []string
	Workers            int
	Timeout            time.Duration
	Verbose            bool
	Run                string
	JSON               bool
	DryRun             bool
	Threshold          float64
	SkipErrPropagation bool
	Diff               string
}

// Validate returns an error if any field has an invalid value.
func Validate(c Config) error {
	if c.Workers <= 0 {
		return fmt.Errorf("-workers must be > 0, got %d", c.Workers)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("-timeout must be > 0, got %s", c.Timeout)
	}
	if c.Threshold < 0 || c.Threshold > 100 {
		return fmt.Errorf("-threshold must be between 0 and 100, got %.1f", c.Threshold)
	}
	return nil
}
