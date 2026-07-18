package rules

import (
	"fmt"

	"github.com/emphity1/reap/internal/parser"
)

// NoPDB flags model-server workloads with no PodDisruptionBudget selecting
// their pods in the linted input: a node drain can evict every replica at
// once, taking the service down during routine maintenance.
//
// Selector matching implements matchLabels only (subset match against the
// pod template labels). A PDB using matchExpressions is assumed to match —
// suppression errs on acceptance rather than faking an evaluation we don't
// implement yet.
type NoPDB struct{}

func (NoPDB) ID() string { return "no-pdb" }

func (NoPDB) Severity() Severity { return Info }

func (r NoPDB) CheckAll(objs []parser.Object, idx *Index) []Finding {
	var findings []Finding
	for _, obj := range objs {
		if _, ok := servedModelServer(obj); !ok {
			continue
		}
		if hasPDB(obj, idx) {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   r.ID(),
			Severity: r.Severity(),
			Message: fmt.Sprintf("model server %s %q has no PodDisruptionBudget selecting its pods in the linted input; "+
				"a node drain can evict every replica at once — downtime during routine maintenance", obj.Kind, obj.Name),
			ObjectRef: obj.Ref(),
			Source:    obj.Source,
			Fix: "add a PDB with minAvailable (or maxUnavailable) whose selector matches the workload's pod labels; " +
				"if the PDB lives outside these manifests, baseline this finding",
		})
	}
	return findings
}

func hasPDB(obj parser.Object, idx *Index) bool {
	labelsVal, _ := obj.Lookup("spec", "template", "metadata", "labels")
	podLabels, _ := labelsVal.(map[string]any)
	for _, pdb := range idx.PDBsInNamespace(obj.Namespace) {
		selVal, ok := pdb.Lookup("spec", "selector")
		if !ok {
			continue
		}
		sel, ok := selVal.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := sel["matchExpressions"]; ok {
			return true // not evaluated: assumed to match
		}
		matchLabels, ok := sel["matchLabels"].(map[string]any)
		if !ok || len(matchLabels) == 0 {
			continue
		}
		if labelsSubset(matchLabels, podLabels) {
			return true
		}
	}
	return false
}

// labelsSubset reports whether every entry of want is present in have with
// an equal value.
func labelsSubset(want, have map[string]any) bool {
	for k, v := range want {
		hv, ok := have[k]
		if !ok || fmt.Sprint(hv) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}
