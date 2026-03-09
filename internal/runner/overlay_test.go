package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNewOverlayManager(t *testing.T) {
	om, err := NewOverlayManager()
	if err != nil {
		t.Fatalf("NewOverlayManager() error: %v", err)
	}
	defer om.Close()

	if om.tempDir == "" {
		t.Error("tempDir should not be empty")
	}
	if _, err := os.Stat(om.tempDir); os.IsNotExist(err) {
		t.Errorf("tempDir %q should exist", om.tempDir)
	}
}

func TestOverlayManager_CreateOverlay(t *testing.T) {
	om, err := NewOverlayManager()
	if err != nil {
		t.Fatalf("NewOverlayManager() error: %v", err)
	}
	defer om.Close()

	originalPath := "/some/project/main.go"
	mutatedSource := []byte("package main\n\nfunc main() {}\n")

	overlayPath, cleanup, err := om.CreateOverlay(originalPath, mutatedSource)
	if err != nil {
		t.Fatalf("CreateOverlay() error: %v", err)
	}
	defer cleanup()

	// Overlay file should exist
	if _, err := os.Stat(overlayPath); os.IsNotExist(err) {
		t.Fatal("overlay JSON file should exist")
	}

	// Read and validate overlay JSON
	data, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatalf("reading overlay: %v", err)
	}

	var overlay overlayJSON
	if err := json.Unmarshal(data, &overlay); err != nil {
		t.Fatalf("unmarshaling overlay: %v", err)
	}

	if len(overlay.Replace) != 1 {
		t.Fatalf("overlay Replace has %d entries, want 1", len(overlay.Replace))
	}

	mutatedFile, ok := overlay.Replace[originalPath]
	if !ok {
		t.Fatalf("overlay Replace missing key %q", originalPath)
	}

	// Verify mutated source was written correctly
	mutatedContent, err := os.ReadFile(mutatedFile)
	if err != nil {
		t.Fatalf("reading mutated file: %v", err)
	}
	if string(mutatedContent) != string(mutatedSource) {
		t.Errorf("mutated content = %q, want %q", string(mutatedContent), string(mutatedSource))
	}
}

func TestOverlayManager_CreateOverlay_MultipleMutations(t *testing.T) {
	om, err := NewOverlayManager()
	if err != nil {
		t.Fatalf("NewOverlayManager() error: %v", err)
	}
	defer om.Close()

	// Create multiple overlays and verify each gets a unique subdirectory
	var overlayPaths []string
	var cleanups []func()

	for i := 0; i < 5; i++ {
		path, cleanup, err := om.CreateOverlay("/project/test.go", []byte("package p"))
		if err != nil {
			t.Fatalf("CreateOverlay(%d) error: %v", i, err)
		}
		overlayPaths = append(overlayPaths, path)
		cleanups = append(cleanups, cleanup)
	}
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()

	// All paths should be unique
	seen := make(map[string]bool)
	for _, p := range overlayPaths {
		if seen[p] {
			t.Errorf("duplicate overlay path: %q", p)
		}
		seen[p] = true
	}
}

func TestOverlayManager_CreateOverlay_ConcurrentSafety(t *testing.T) {
	om, err := NewOverlayManager()
	if err != nil {
		t.Fatalf("NewOverlayManager() error: %v", err)
	}
	defer om.Close()

	const goroutines = 20
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			path, cleanup, err := om.CreateOverlay("/project/main.go", []byte("package main"))
			if err != nil {
				errs <- err
				return
			}
			defer cleanup()
			// Verify the overlay file exists
			if _, err := os.Stat(path); os.IsNotExist(err) {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent CreateOverlay error: %v", err)
	}
}

func TestOverlayManager_Close(t *testing.T) {
	om, err := NewOverlayManager()
	if err != nil {
		t.Fatalf("NewOverlayManager() error: %v", err)
	}

	tempDir := om.tempDir

	// Create an overlay
	_, _, err = om.CreateOverlay("/project/test.go", []byte("package p"))
	if err != nil {
		t.Fatalf("CreateOverlay() error: %v", err)
	}

	// Close should remove the temp dir
	if err := om.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Errorf("tempDir %q should be removed after Close()", tempDir)
	}
}

func TestOverlayManager_CreateOverlay_Cleanup(t *testing.T) {
	om, err := NewOverlayManager()
	if err != nil {
		t.Fatalf("NewOverlayManager() error: %v", err)
	}
	defer om.Close()

	overlayPath, cleanup, err := om.CreateOverlay("/project/test.go", []byte("package p"))
	if err != nil {
		t.Fatalf("CreateOverlay() error: %v", err)
	}

	// Get the subdirectory containing the overlay
	subDir := filepath.Dir(overlayPath)

	// Verify it exists
	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Fatal("subdirectory should exist before cleanup")
	}

	// Run cleanup
	cleanup()

	// Verify subdirectory is removed
	if _, err := os.Stat(subDir); !os.IsNotExist(err) {
		t.Error("subdirectory should be removed after cleanup")
	}
}

func TestOverlayManager_CreateOverlay_EmptySource(t *testing.T) {
	om, err := NewOverlayManager()
	if err != nil {
		t.Fatalf("NewOverlayManager() error: %v", err)
	}
	defer om.Close()

	// Empty source should still work
	overlayPath, cleanup, err := om.CreateOverlay("/project/test.go", []byte{})
	if err != nil {
		t.Fatalf("CreateOverlay() error: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(overlayPath); os.IsNotExist(err) {
		t.Fatal("overlay file should exist even with empty source")
	}
}

func TestOverlayManager_CreateOverlay_LargeSource(t *testing.T) {
	om, err := NewOverlayManager()
	if err != nil {
		t.Fatalf("NewOverlayManager() error: %v", err)
	}
	defer om.Close()

	// 1MB source
	src := make([]byte, 1024*1024)
	for i := range src {
		src[i] = 'x'
	}

	overlayPath, cleanup, err := om.CreateOverlay("/project/big.go", src)
	if err != nil {
		t.Fatalf("CreateOverlay() error: %v", err)
	}
	defer cleanup()

	// Read back the overlay and verify mutated file size
	data, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatalf("reading overlay: %v", err)
	}
	var overlay overlayJSON
	json.Unmarshal(data, &overlay)

	mutatedFile := overlay.Replace["/project/big.go"]
	content, err := os.ReadFile(mutatedFile)
	if err != nil {
		t.Fatalf("reading mutated file: %v", err)
	}
	if len(content) != len(src) {
		t.Errorf("mutated file size = %d, want %d", len(content), len(src))
	}
}

func TestOverlayManager_CreateOverlay_PreservesFilename(t *testing.T) {
	om, err := NewOverlayManager()
	if err != nil {
		t.Fatalf("NewOverlayManager() error: %v", err)
	}
	defer om.Close()

	originalPath := "/deep/nested/path/myfile.go"
	overlayPath, cleanup, err := om.CreateOverlay(originalPath, []byte("package p"))
	if err != nil {
		t.Fatalf("CreateOverlay() error: %v", err)
	}
	defer cleanup()

	// Read the overlay JSON
	data, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatalf("reading overlay: %v", err)
	}
	var overlay overlayJSON
	json.Unmarshal(data, &overlay)

	mutatedFile := overlay.Replace[originalPath]
	// The mutated file should have the same base filename
	if filepath.Base(mutatedFile) != "myfile.go" {
		t.Errorf("mutated file base = %q, want %q", filepath.Base(mutatedFile), "myfile.go")
	}
}
