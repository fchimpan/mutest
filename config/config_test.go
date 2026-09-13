package config

import (
	"math"
	"testing"
	"time"
)

func TestRejectNonFiniteThreshold(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if Validate(Config{Workers: 1, Timeout: time.Second, Threshold: v}) == nil {
			t.Fatalf("accepted %v", v)
		}
	}
}

func TestThresholdBoundaries(t *testing.T) {
	for _, tt := range []struct {
		threshold float64
		valid     bool
	}{
		{
			threshold: 0,
			valid:     true,
		},
		{
			threshold: 100,
			valid:     true,
		},
		{
			threshold: math.Nextafter(0, -1),
		},
		{
			threshold: math.Nextafter(100, 101),
		},
	} {
		err := Validate(Config{Workers: 1, Timeout: time.Second, Threshold: tt.threshold})
		if (err == nil) != tt.valid {
			t.Errorf("threshold %v: %v", tt.threshold, err)
		}
	}
}
