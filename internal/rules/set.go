package rules

import "github.com/emphity1/reap/internal/parser"

// SetRule checks the whole set of parsed objects at once, for rules whose
// evidence lives in a different object than the one at fault (an HPA
// references a Deployment; a PDB selects pods by label).
//
// Set-level rules judge only the linted input: the missing object may simply
// live in another repo. They therefore ship at info severity, say "in the
// linted input" in their messages, and lean on the baseline mechanism.
type SetRule interface {
	ID() string
	Severity() Severity
	CheckAll(objs []parser.Object, idx *Index) []Finding
}

// AllSet returns every registered set-level rule.
func AllSet() []SetRule {
	return []SetRule{
		InferenceNoHPA{},
		NoPDB{},
	}
}
