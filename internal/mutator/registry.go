package mutator

// DefaultMutators returns the default set of mutators.
// This is the "DEFAULTS" group — a pragmatic balance of signal and noise.
func DefaultMutators() []Mutator {
	return []Mutator{
		NewArithmeticMutator(),
		NewConditionalMutator(),
		NewLogicalMutator(),
		NewNegateMutator(),
		NewReturnValueMutator(),
		NewStatementMutator(),
		NewBranchBodyMutator(),
		NewLoopBreakMutator(),
	}
}

// MutatorsByName returns a map of mutator name → Mutator for all available mutators.
func MutatorsByName() map[string]Mutator {
	all := DefaultMutators()
	m := make(map[string]Mutator, len(all))
	for _, mut := range all {
		m[mut.Name()] = mut
	}
	return m
}

// SelectMutators filters mutators by name. If names is empty, returns all defaults.
func SelectMutators(names []string) []Mutator {
	if len(names) == 0 {
		return DefaultMutators()
	}
	byName := MutatorsByName()
	var selected []Mutator
	for _, name := range names {
		if m, ok := byName[name]; ok {
			selected = append(selected, m)
		}
	}
	return selected
}
