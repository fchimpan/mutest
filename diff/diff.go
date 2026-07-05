package diff

import "github.com/fchimpan/mutest/mutator"

// ChangedLines maps absolute file paths to sets of changed line numbers.
// A nil line set means the whole file is considered changed (used for
// untracked files, where every line is new); a non-nil set restricts
// matches to exactly those lines.
type ChangedLines map[string]map[int]bool

// FilterPoints returns only the MutationPoints whose (File, Line)
// falls within the changed lines. If cl is nil, all points are returned
// unchanged. A file mapped to a nil line set matches all of its points
// (whole-file change).
func FilterPoints(points []mutator.MutationPoint, cl ChangedLines) []mutator.MutationPoint {
	if cl == nil {
		return points
	}
	var filtered []mutator.MutationPoint
	for _, p := range points {
		if lines, ok := cl[p.File]; ok && (lines == nil || lines[p.Line]) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}
