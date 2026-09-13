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
