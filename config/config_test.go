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
		{0, true}, {100, true}, {math.Nextafter(0, -1), false}, {math.Nextafter(100, 101), false},
	} {
		err := Validate(Config{Workers: 1, Timeout: time.Second, Threshold: tt.threshold})
		if (err == nil) != tt.valid {
			t.Errorf("threshold %v: %v", tt.threshold, err)
		}
	}
}
