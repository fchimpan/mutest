package process

import (
	"bytes"
	"testing"
)

func TestTailBufferBoundsMemory(t *testing.T) {
	b := &tailBuffer{limit: 16}
	chunks := [][]byte{
		bytes.Repeat([]byte("x"), 10000),
		[]byte("abcdefghijkl"),
		[]byte("mnop"),
	}
	for _, p := range chunks {
		if n, err := b.Write(p); err != nil || n != len(p) {
			t.Fatal(n, err)
		}
	}
	if string(b.data) != "abcdefghijklmnop" || !b.truncated {
		t.Fatalf("%q truncated=%v", b.data, b.truncated)
	}
}

func TestTailBufferExactCapacityIsNotTruncated(t *testing.T) {
	tests := []struct {
		name   string
		chunks [][]byte
	}{
		{
			name: "single write",
			chunks: [][]byte{
				[]byte("12345678"),
			},
		},
		{
			name: "multiple writes",
			chunks: [][]byte{
				[]byte("1234"),
				[]byte("5678"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &tailBuffer{limit: 8}
			for _, p := range tt.chunks {
				if _, err := b.Write(p); err != nil {
					t.Fatal(err)
				}
			}
			if b.truncated || string(b.data) != "12345678" {
				t.Fatalf("exact capacity: %q truncated=%v", b.data, b.truncated)
			}
		})
	}
}
