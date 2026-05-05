package mutest

import (
	"encoding/json"
	"io"
	"math"
	"time"

	"github.com/fchimpan/mutest/runner"
)

type jsonMutationPoint struct {
	File     string `json:"file"`
	Package  string `json:"package"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Original string `json:"original"`
	Mutated  string `json:"mutated"`
	Desc     string `json:"desc"`
}

type jsonResult struct {
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

type jsonSummary struct {
	Total    int          `json:"total"`
	Killed   int          `json:"killed"`
	TimedOut int          `json:"timed_out"`
	Survived int          `json:"survived"`
	Errors   int          `json:"errors"`
	KillRate float64      `json:"kill_rate"`
	Duration string       `json:"duration"`
	Results  []jsonResult `json:"results"`
}

func toJSONResult(r runner.Result, rpc *relPathCache) jsonResult {
	status := "survived"
	if r.Err != nil {
		status = "error"
	} else if r.TimedOut {
		status = "timeout"
	} else if r.Killed {
		status = "killed"
	}
	jr := jsonResult{
		Status:   status,
		File:     rpc.get(r.Point.File),
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

func writeJSONSummary(w io.Writer, s *runner.Summary, rpc *relPathCache, includeResults bool) {
	killRate := calcKillRate(s)

	var results []jsonResult
	if includeResults {
		results = make([]jsonResult, len(s.Results))
		for i, r := range s.Results {
			results[i] = toJSONResult(r, rpc)
		}
	}

	summary := jsonSummary{
		Total:    s.Total,
		Killed:   s.Killed,
		TimedOut: s.TimedOut,
		Survived: s.Survived,
		Errors:   s.Errors,
		KillRate: math.Round(killRate*10) / 10,
		Duration: s.Duration.Round(time.Millisecond).String(),
		Results:  results,
	}
	enc := newJSONEncoder(w)
	enc.Encode(summary)
}

func newJSONEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc
}
