// Package runner handles test execution against mutated source code using Go's overlay mechanism.
package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
)

// overlayJSON is the Go overlay file format.
type overlayJSON struct {
	Replace map[string]string `json:"Replace"`
}

// OverlayManager manages temporary files for the overlay mechanism.
// Each instance is goroutine-safe and creates isolated temp files.
type OverlayManager struct {
	tempDir string
	counter atomic.Int64
}

// NewOverlayManager creates a new overlay manager with an isolated temp directory.
func NewOverlayManager() (*OverlayManager, error) {
	dir, err := os.MkdirTemp("", "mutest-overlay-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	return &OverlayManager{tempDir: dir}, nil
}

// CreateOverlay writes the mutated source to a temp file and returns
// the path to an overlay JSON file that maps the original → mutated.
func (om *OverlayManager) CreateOverlay(originalPath string, mutatedSource []byte) (overlayPath string, cleanup func(), err error) {
	id := om.counter.Add(1)
	subDir := filepath.Join(om.tempDir, fmt.Sprintf("m%d", id))
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("creating mutant dir: %w", err)
	}

	mutatedFile := filepath.Join(subDir, filepath.Base(originalPath))
	if err := os.WriteFile(mutatedFile, mutatedSource, 0o644); err != nil {
		return "", nil, fmt.Errorf("writing mutated source: %w", err)
	}

	overlay := overlayJSON{
		Replace: map[string]string{
			originalPath: mutatedFile,
		},
	}
	overlayData, err := json.Marshal(overlay)
	if err != nil {
		return "", nil, fmt.Errorf("marshaling overlay: %w", err)
	}

	overlayFile := filepath.Join(subDir, "overlay.json")
	if err := os.WriteFile(overlayFile, overlayData, 0o644); err != nil {
		return "", nil, fmt.Errorf("writing overlay file: %w", err)
	}

	cleanup = func() { os.RemoveAll(subDir) }
	return overlayFile, cleanup, nil
}

// Close removes the entire temp directory.
func (om *OverlayManager) Close() error {
	return os.RemoveAll(om.tempDir)
}
