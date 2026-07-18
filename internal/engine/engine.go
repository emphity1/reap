// Package engine runs every rule over every object and collects findings.
package engine

import (
	"sort"

	"github.com/emphity1/reap/internal/parser"
	"github.com/emphity1/reap/internal/rules"
)

// Run applies each rule to each object and returns the findings sorted by
// source, then object, then rule, so output is stable and grouped per file.
func Run(objs []parser.Object, rs []rules.Rule) []rules.Finding {
	var findings []rules.Finding
	for _, obj := range objs {
		for _, r := range rs {
			findings = append(findings, r.Check(obj)...)
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.ObjectRef != b.ObjectRef {
			return a.ObjectRef < b.ObjectRef
		}
		return a.RuleID < b.RuleID
	})
	return findings
}
