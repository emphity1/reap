package rules

import "github.com/emphity1/reap/internal/parser"

// Index provides the cross-object lookups set-level rules need. The engine
// builds it once per run instead of letting each rule rescan the object
// list.
type Index struct {
	hpasByTarget map[string][]parser.Object
	pdbsByNS     map[string][]parser.Object
}

// BuildIndex scans the parsed objects for the cross-referencing kinds.
func BuildIndex(objs []parser.Object) *Index {
	idx := &Index{
		hpasByTarget: map[string][]parser.Object{},
		pdbsByNS:     map[string][]parser.Object{},
	}
	for _, obj := range objs {
		switch obj.Kind {
		case "HorizontalPodAutoscaler":
			kind, _ := obj.Lookup("spec", "scaleTargetRef", "kind")
			name, _ := obj.Lookup("spec", "scaleTargetRef", "name")
			k, kOK := kind.(string)
			n, nOK := name.(string)
			if kOK && nOK {
				key := hpaTargetKey(k, obj.Namespace, n)
				idx.hpasByTarget[key] = append(idx.hpasByTarget[key], obj)
			}
		case "PodDisruptionBudget":
			idx.pdbsByNS[obj.Namespace] = append(idx.pdbsByNS[obj.Namespace], obj)
		}
	}
	return idx
}

// HPAsTargeting returns the HPAs whose scaleTargetRef points at the given
// workload. Namespaces compare verbatim: empty stays empty.
func (idx *Index) HPAsTargeting(kind, namespace, name string) []parser.Object {
	return idx.hpasByTarget[hpaTargetKey(kind, namespace, name)]
}

// PDBsInNamespace returns the PodDisruptionBudgets declared in the given
// namespace.
func (idx *Index) PDBsInNamespace(namespace string) []parser.Object {
	return idx.pdbsByNS[namespace]
}

func hpaTargetKey(kind, namespace, name string) string {
	return kind + "\x00" + namespace + "\x00" + name
}
