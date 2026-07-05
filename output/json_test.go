package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/fchimpan/mutest/mutator"
	"github.com/fchimpan/mutest/runner"
)

func TestWriteJSONSummary(t *testing.T) {
	var buf bytes.Buffer
	summary := &runner.Summary{
		Total:    4,
		Killed:   3,
		Survived: 1,
		Errors:   0,
		Duration: 1234 * time.Millisecond,
	}

	WriteJSONSummary(&buf, summary, NewRelPathCache("/base"), true)

	var js JSONSummary
	if err := json.Unmarshal(buf.Bytes(), &js); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if js.Total != 4 {
		t.Errorf("expected total 4, got %d", js.Total)
	}
	if js.KillRate != 75.0 {
		t.Errorf("expected kill rate 75.0, got %f", js.KillRate)
	}
	if js.Duration != "1.234s" {
		t.Errorf("expected duration '1.234s', got %s", js.Duration)
	}
}

func TestToJSONResult(t *testing.T) {
	rpc := NewRelPathCache("/work")

	t.Run("killed", func(t *testing.T) {
		r := runner.Result{
			Point:    mutator.MutationPoint{File: "/work/foo.go", Package: "foo", Line: 10, Column: 5},
			Killed:   true,
			Duration: 123 * time.Millisecond,
		}
		jr := ToJSONResult(r, rpc)
		if jr.Status != "killed" {
			t.Errorf("expected status killed, got %s", jr.Status)
		}
		if jr.File != "foo.go" {
			t.Errorf("expected relative path foo.go, got %s", jr.File)
		}
		if jr.Error != "" {
			t.Error("killed result should have no error")
		}
	})

	t.Run("survived", func(t *testing.T) {
		r := runner.Result{
			Point:    mutator.MutationPoint{File: "/work/bar.go"},
			Killed:   false,
			Duration: 50 * time.Millisecond,
		}
		jr := ToJSONResult(r, rpc)
		if jr.Status != "survived" {
			t.Errorf("expected status survived, got %s", jr.Status)
		}
	})

	t.Run("error", func(t *testing.T) {
		r := runner.Result{
			Point:    mutator.MutationPoint{File: "/work/baz.go"},
			Err:      fmt.Errorf("prepare failed"),
			Duration: 1 * time.Millisecond,
		}
		jr := ToJSONResult(r, rpc)
		if jr.Status != "error" {
			t.Errorf("expected status error, got %s", jr.Status)
		}
		if jr.Error != "prepare failed" {
			t.Errorf("expected error message, got %s", jr.Error)
		}
	})

	t.Run("timed_out", func(t *testing.T) {
		r := runner.Result{
			Point:    mutator.MutationPoint{File: "/work/timeout.go"},
			Killed:   false,
			TimedOut: true,
			Duration: 30 * time.Second,
		}
		jr := ToJSONResult(r, rpc)
		if jr.Status != "timeout" {
			t.Errorf("expected status timeout, got %s", jr.Status)
		}
		if !jr.TimedOut {
			t.Error("expected timed_out to be true")
		}
	})

	t.Run("canceled", func(t *testing.T) {
		r := runner.Result{
			Point:    mutator.MutationPoint{File: "/work/canceled.go"},
			Canceled: true,
			Duration: 2 * time.Millisecond,
		}
		jr := ToJSONResult(r, rpc)
		if jr.Status != "canceled" {
			t.Errorf("expected status canceled, got %s", jr.Status)
		}
		if !jr.Canceled {
			t.Error("expected canceled to be true")
		}
	})
}

func TestWriteJSONSummary_Canceled(t *testing.T) {
	var buf bytes.Buffer
	summary := &runner.Summary{
		Total:    3,
		Killed:   1,
		Canceled: 2,
		Duration: time.Second,
	}

	WriteJSONSummary(&buf, summary, NewRelPathCache("/base"), true)

	var js JSONSummary
	if err := json.Unmarshal(buf.Bytes(), &js); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if js.Canceled != 2 {
		t.Errorf("expected canceled 2, got %d", js.Canceled)
	}
}
