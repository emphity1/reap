// Package engine runs every rule over every object and collects findings.
package engine

import (
	"sort"

	"github.com/emphity1/reap/internal/parser"
	"github.com/emphity1/reap/internal/rules"
)

// Run applies each per-object rule to each object, then each set-level rule
// to the whole set (with the cross-object Index built once), and returns the
// findings sorted by source, then object, then severity (highest first, so a
// reader sees what matters before the info tail), then rule, so output is
// stable and grouped per file.
func Run(objs []parser.Object, rs []rules.Rule, srs []rules.SetRule) []rules.Finding {
	var findings []rules.Finding
	for _, obj := range objs {
		for _, r := range rs {
			findings = append(findings, r.Check(obj)...)
		}
	}
	if len(srs) > 0 {
		idx := rules.BuildIndex(objs)
		for _, sr := range srs {
			findings = append(findings, sr.CheckAll(objs, idx)...)
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
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		return a.RuleID < b.RuleID
	})
	return findings
}
