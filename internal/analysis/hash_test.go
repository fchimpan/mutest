package analysis

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFileHash_BasicFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := FileHash(path)
	if err != nil {
		t.Fatalf("FileHash() error: %v", err)
	}

	h := sha256.Sum256(content)
	want := fmt.Sprintf("%x", h[:])
	if got != want {
		t.Errorf("FileHash() = %q, want %q", got, want)
	}
}

func TestFileHash_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := FileHash(path)
	if err != nil {
		t.Fatalf("FileHash() error: %v", err)
	}

	h := sha256.Sum256([]byte{})
	want := fmt.Sprintf("%x", h[:])
	if got != want {
		t.Errorf("FileHash(empty) = %q, want %q", got, want)
	}
}

func TestFileHash_NonexistentFile(t *testing.T) {
	_, err := FileHash("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestFileHash_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")
	// 1MB file
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := FileHash(path)
	if err != nil {
		t.Fatalf("FileHash() error: %v", err)
	}

	h := sha256.Sum256(data)
	want := fmt.Sprintf("%x", h[:])
	if got != want {
		t.Errorf("FileHash(large) = %q, want %q", got, want)
	}
}

func TestFileHash_DifferentContentsDifferentHashes(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "file1.txt")
	path2 := filepath.Join(dir, "file2.txt")
	if err := os.WriteFile(path1, []byte("content A"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(path2, []byte("content B"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	hash1, err := FileHash(path1)
	if err != nil {
		t.Fatalf("FileHash(path1) error: %v", err)
	}
	hash2, err := FileHash(path2)
	if err != nil {
		t.Fatalf("FileHash(path2) error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("different contents should produce different hashes")
	}
}

func TestFileHash_SameContentsSameHash(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "file1.txt")
	path2 := filepath.Join(dir, "file2.txt")
	content := []byte("identical content")
	if err := os.WriteFile(path1, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(path2, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	hash1, err := FileHash(path1)
	if err != nil {
		t.Fatalf("FileHash(path1) error: %v", err)
	}
	hash2, err := FileHash(path2)
	if err != nil {
		t.Fatalf("FileHash(path2) error: %v", err)
	}

	if hash1 != hash2 {
		t.Error("same contents should produce same hashes")
	}
}

func TestFileHash_HashLength(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := FileHash(path)
	if err != nil {
		t.Fatalf("FileHash() error: %v", err)
	}

	// SHA-256 produces 32 bytes = 64 hex characters
	if len(got) != 64 {
		t.Errorf("hash length = %d, want 64", len(got))
	}
}

func TestFileHash_BinaryContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	// Include null bytes and all byte values
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := FileHash(path)
	if err != nil {
		t.Fatalf("FileHash() error: %v", err)
	}

	h := sha256.Sum256(data)
	want := fmt.Sprintf("%x", h[:])
	if got != want {
		t.Errorf("FileHash(binary) = %q, want %q", got, want)
	}
}
