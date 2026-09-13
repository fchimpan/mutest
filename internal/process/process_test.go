package process

import (
	"bytes"
	"testing"
)

func TestTailBufferBoundsMemory(t *testing.T) {
	b := &tailBuffer{limit: 16}
	for _, p := range [][]byte{bytes.Repeat([]byte("x"), 10000), []byte("abcdefghijkl"), []byte("mnop")} {
		if n, err := b.Write(p); err != nil || n != len(p) {
			t.Fatal(n, err)
		}
	}
	if string(b.data) != "abcdefghijklmnop" || !b.truncated {
		t.Fatalf("%q truncated=%v", b.data, b.truncated)
	}
}
