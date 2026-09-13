package main

import (
	"fmt"
	"github.com/fchimpan/mutest/cmd/mutest"
	"testing"
)

func TestExitCodes(t *testing.T) {
	for _, tt := range []struct {
		err  error
		code int
	}{{nil, 0}, {mutest.ErrTestsFailed, 1}, {mutest.ErrBaseline, 1}, {mutest.ErrInterrupted, 1}, {mutest.ErrDiscovery, 2}, {mutest.ErrBuild, 2}, {fmt.Errorf("wrapped: %w", mutest.ErrDiscovery), 2}} {
		if got := exitCode(tt.err); got != tt.code {
			t.Errorf("%v: %d want %d", tt.err, got, tt.code)
		}
	}
}
