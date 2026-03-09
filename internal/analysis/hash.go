// Package analysis provides code analysis utilities for mutation testing.
package analysis

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// FileHash computes the SHA-256 hash of a file's contents.
func FileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file for hash: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
