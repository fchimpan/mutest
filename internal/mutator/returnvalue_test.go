package mutator

import (
	"testing"
)

func TestReturnValueMutator_Name(t *testing.T) {
	m := NewReturnValueMutator()
	if m.Name() != "return_value" {
		t.Errorf("Name() = %q, want %q", m.Name(), "return_value")
	}
}

func TestReturnValueMutator_Mutate(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name:  "func returning int",
			src:   `package p; func f() int { return 42 }`,
			wantN: 2, // 0 and 1
		},
		{
			name:  "func returning bool",
			src:   `package p; func f() bool { return true }`,
			wantN: 2, // false and true
		},
		{
			name:  "func returning string",
			src:   `package p; func f() string { return "hello" }`,
			wantN: 2, // "" and "mutest"
		},
		{
			name:  "func returning error",
			src:   `package p; import "fmt"; func f() error { return fmt.Errorf("x") }`,
			wantN: 2, // nil and fmt.Errorf("mutest")
		},
		{
			name:  "func returning (int, error)",
			src:   `package p; import "fmt"; func f() (int, error) { return 1, nil }`,
			wantN: 2, // (0, nil) and (1, fmt.Errorf)
		},
		{
			name:  "func returning pointer",
			src:   `package p; type T struct{}; func f() *T { return &T{} }`,
			wantN: 1, // nil (zero and nonzero same)
		},
		{
			name:  "func returning slice",
			src:   `package p; func f() []int { return []int{1,2} }`,
			wantN: 2, // nil and []int{}
		},
		{
			name:  "func returning map",
			src:   `package p; func f() map[string]int { return nil }`,
			wantN: 1, // nil only
		},
		{
			name:  "func returning float64",
			src:   `package p; func f() float64 { return 3.14 }`,
			wantN: 2, // 0.0 and 1.0
		},
		{
			name:  "func returning byte",
			src:   `package p; func f() byte { return 'a' }`,
			wantN: 2, // 0 and 1
		},
		{
			name:  "func returning interface",
			src:   `package p; func f() interface{} { return nil }`,
			wantN: 1, // nil only
		},
		// Edge cases: functions that should NOT be mutated
		{
			name:  "no return values",
			src:   `package p; func f() { println("hi") }`,
			wantN: 0,
		},
		{
			name:  "no body",
			src:   `package p; type I interface { M() int }`,
			wantN: 0,
		},
		{
			name:  "Test function skipped",
			src:   `package p; func TestFoo() int { return 1 }`,
			wantN: 0,
		},
		{
			name:  "Benchmark function skipped",
			src:   `package p; func BenchmarkFoo() int { return 1 }`,
			wantN: 0,
		},
		{
			name:  "main function skipped",
			src:   `package p; func main() int { return 0 }`,
			wantN: 0,
		},
		{
			name:  "init function skipped",
			src:   `package p; func init() int { return 0 }`,
			wantN: 0,
		},
		{
			name:  "method with receiver",
			src:   `package p; type T struct{}; func (t T) Foo() int { return 1 }`,
			wantN: 2,
		},
		{
			name:  "multiple named returns",
			src:   `package p; func f() (a, b int) { return 1, 2 }`,
			wantN: 2, // (0, 0) and (1, 1)
		},
	}

	m := NewReturnValueMutator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset, file := mustParse(t, tt.src)
			mutations := collectMutations(t, m, fset, file)
			if len(mutations) != tt.wantN {
				for i, mut := range mutations {
					t.Logf("  mutation[%d]: %s -> %s", i, mut.Original, mut.Mutated)
				}
				t.Fatalf("got %d mutations, want %d", len(mutations), tt.wantN)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 3, "hel..."},
		{"", 5, ""},
		{"ab", 0, "..."}, // max=0: s[:0] + "..." = "..."
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}
