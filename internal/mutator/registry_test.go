package mutator

import (
	"testing"
)

func TestDefaultMutators(t *testing.T) {
	mutators := DefaultMutators()
	if len(mutators) != 8 {
		t.Fatalf("DefaultMutators() returned %d mutators, want 8", len(mutators))
	}
	expectedNames := []string{
		"arithmetic", "conditional", "logical", "negate_removal",
		"return_value", "statement", "branch_body", "loop_break",
	}
	for i, m := range mutators {
		if m.Name() != expectedNames[i] {
			t.Errorf("mutator[%d].Name() = %q, want %q", i, m.Name(), expectedNames[i])
		}
	}
}

func TestMutatorsByName(t *testing.T) {
	byName := MutatorsByName()
	if len(byName) != 8 {
		t.Fatalf("MutatorsByName() returned %d entries, want 8", len(byName))
	}
	for name, m := range byName {
		if m.Name() != name {
			t.Errorf("MutatorsByName()[%q].Name() = %q", name, m.Name())
		}
	}
}

func TestSelectMutators(t *testing.T) {
	t.Run("nil returns all defaults", func(t *testing.T) {
		got := SelectMutators(nil)
		if len(got) != 8 {
			t.Fatalf("SelectMutators(nil) returned %d, want 8", len(got))
		}
	})

	t.Run("empty slice returns all defaults", func(t *testing.T) {
		got := SelectMutators([]string{})
		if len(got) != 8 {
			t.Fatalf("SelectMutators([]) returned %d, want 8", len(got))
		}
	})

	t.Run("select specific mutators", func(t *testing.T) {
		got := SelectMutators([]string{"arithmetic", "logical"})
		if len(got) != 2 {
			t.Fatalf("got %d, want 2", len(got))
		}
		if got[0].Name() != "arithmetic" || got[1].Name() != "logical" {
			t.Errorf("got [%s, %s], want [arithmetic, logical]", got[0].Name(), got[1].Name())
		}
	})

	t.Run("unknown name ignored", func(t *testing.T) {
		got := SelectMutators([]string{"arithmetic", "nonexistent"})
		if len(got) != 1 {
			t.Fatalf("got %d, want 1", len(got))
		}
	})

	t.Run("all unknown names returns empty", func(t *testing.T) {
		got := SelectMutators([]string{"foo", "bar"})
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})
}
