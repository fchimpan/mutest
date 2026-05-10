package output

import (
	"encoding/json"
	"io"
	"math"
	"time"

	"github.com/fchimpan/mutest/mutator"
	"github.com/fchimpan/mutest/runner"
)

// JSONMutationPoint is the JSON wire format for a discovered mutation point.
type JSONMutationPoint struct {
	File     string `json:"file"`
	Package  string `json:"package"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Original string `json:"original"`
	Mutated  string `json:"mutated"`
	Desc     string `json:"desc"`
}

// JSONResult is the JSON wire format for a single mutant test result.
type JSONResult struct {
	Status   string `json:"status"`
	File     string `json:"file"`
	Package  string `json:"package"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Original string `json:"original"`
	Mutated  string `json:"mutated"`
	Desc     string `json:"desc"`
	Duration string `json:"duration"`
	TimedOut bool   `json:"timed_out,omitempty"`
	Error    string `json:"error,omitempty"`
}

// JSONSummary is the JSON wire format for the aggregate run summary.
type JSONSummary struct {
	Total    int          `json:"total"`
	Killed   int          `json:"killed"`
	TimedOut int          `json:"timed_out"`
	Survived int          `json:"survived"`
	Errors   int          `json:"errors"`
	KillRate float64      `json:"kill_rate"`
	Duration string       `json:"duration"`
	Results  []JSONResult `json:"results"`
}

// ToJSONResult converts a runner.Result into the JSON wire format.
func ToJSONResult(r runner.Result, rpc *RelPathCache) JSONResult {
	status := "survived"
	if r.Err != nil {
		status = "error"
	} else if r.TimedOut {
		status = "timeout"
	} else if r.Killed {
		status = "killed"
	}
	jr := JSONResult{
		Status:   status,
		File:     rpc.Get(r.Point.File),
		Package:  r.Point.Package,
		Line:     r.Point.Line,
		Column:   r.Point.Column,
		Original: r.Point.Original.String(),
		Mutated:  r.Point.Mutated.String(),
		Desc:     r.Point.Desc,
		Duration: r.Duration.Round(time.Millisecond).String(),
		TimedOut: r.TimedOut,
	}
	if r.Err != nil {
		jr.Error = r.Err.Error()
	}
	return jr
}

// WriteJSONSummary writes the aggregate JSON summary to w. When includeResults
// is false, the per-mutant Results slice is omitted (used for verbose NDJSON
// streaming where individual results were already emitted).
func WriteJSONSummary(w io.Writer, s *runner.Summary, rpc *RelPathCache, includeResults bool) {
	killRate := CalcKillRate(s)

	var results []JSONResult
	if includeResults {
		results = make([]JSONResult, len(s.Results))
		for i, r := range s.Results {
			results[i] = ToJSONResult(r, rpc)
		}
	}

	summary := JSONSummary{
		Total:    s.Total,
		Killed:   s.Killed,
		TimedOut: s.TimedOut,
		Survived: s.Survived,
		Errors:   s.Errors,
		KillRate: math.Round(killRate*10) / 10,
		Duration: s.Duration.Round(time.Millisecond).String(),
		Results:  results,
	}
	enc := NewJSONEncoder(w)
	enc.Encode(summary)
}

// DryRunJSON writes the discovered mutation points as a JSON array to w.
func DryRunJSON(w io.Writer, points []mutator.MutationPoint, rpc *RelPathCache) {
	pts := make([]JSONMutationPoint, len(points))
	for i, p := range points {
		pts[i] = JSONMutationPoint{
			File:     rpc.Get(p.File),
			Package:  p.Package,
			Line:     p.Line,
			Column:   p.Column,
			Original: p.Original.String(),
			Mutated:  p.Mutated.String(),
			Desc:     p.Desc,
		}
	}
	enc := NewJSONEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(pts)
}

// NewJSONEncoder returns an encoder configured for mutest's JSON output
// (HTML escaping disabled).
func NewJSONEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc
}
