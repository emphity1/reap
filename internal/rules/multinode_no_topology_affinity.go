package rules

import (
	"fmt"

	"github.com/emphity1/reap/internal/parser"
)

// tasAnnotations are Kueue Topology Aware Scheduling markers. Suppression
// only: an unmatched key just means weaker suppression, never a false
// positive.
var tasAnnotations = []string{
	"kueue.x-k8s.io/podset-required-topology",
	"kueue.x-k8s.io/podset-preferred-topology",
}

// MultinodeNoTopologyAffinity flags multi-replica GPU training jobs whose
// pod templates express no placement intent at all. When replicas land
// across racks or zones, NCCL inter-node collectives run over the slow
// network path and training throughput drops — the GPUs are busy waiting,
// which costs the same as busy computing. Reuses the Kubeflow kind coverage
// of the gang-scheduling rule.
type MultinodeNoTopologyAffinity struct{}

func (MultinodeNoTopologyAffinity) ID() string { return "multinode-no-topology-affinity" }

func (MultinodeNoTopologyAffinity) Severity() Severity { return Info }

func (r MultinodeNoTopologyAffinity) Check(obj parser.Object) []Finding {
	specKey, ok := trainingReplicaSpecKeys[obj.Kind]
	if !ok {
		return nil
	}
	rolesVal, ok := obj.Lookup("spec", specKey)
	if !ok {
		return nil
	}
	roles, ok := rolesVal.(map[string]any)
	if !ok {
		return nil
	}
	replicas := totalReplicas(roles)
	if replicas <= 1 {
		return nil
	}
	if gpuSummary(obj.Containers()) == "" {
		return nil
	}
	if hasTopologyPlacement(obj, roles) {
		return nil
	}
	return []Finding{{
		RuleID:   r.ID(),
		Severity: r.Severity(),
		Message: fmt.Sprintf("%s %q runs %d GPU replicas with no topology-aware placement (no pod/node affinity, "+
			"topologySpreadConstraints, nodeSelector, or topology-aware scheduler hints); replicas spread across racks or zones "+
			"push NCCL collectives onto the slow network path, and stalled GPUs cost the same as busy ones",
			obj.Kind, obj.Name, replicas),
		ObjectRef: obj.Ref(),
		Source:    obj.Source,
		Fix: "co-locate the replicas: pod affinity on a topology domain (e.g. topology.kubernetes.io/zone), " +
			"a nodeSelector for a compact placement node group, or Kueue Topology Aware Scheduling",
	}}
}

// hasTopologyPlacement reports whether any role template (or the object
// itself) expresses placement intent. Suppression errs on acceptance.
func hasTopologyPlacement(obj parser.Object, roles map[string]any) bool {
	if meta, ok := obj.Raw["metadata"].(map[string]any); ok {
		if hasAnyKey(meta["annotations"], tasAnnotations) {
			return true
		}
	}
	for _, roleVal := range roles {
		role, ok := roleVal.(map[string]any)
		if !ok {
			continue
		}
		tmpl, ok := role["template"].(map[string]any)
		if !ok {
			continue
		}
		if meta, ok := tmpl["metadata"].(map[string]any); ok {
			if hasAnyKey(meta["annotations"], tasAnnotations) {
				return true
			}
		}
		spec, ok := tmpl["spec"].(map[string]any)
		if !ok {
			continue
		}
		if _, ok := spec["affinity"].(map[string]any); ok {
			return true
		}
		if tsc, ok := spec["topologySpreadConstraints"].([]any); ok && len(tsc) > 0 {
			return true
		}
		if sel, ok := spec["nodeSelector"].(map[string]any); ok && len(sel) > 0 {
			return true
		}
	}
	return false
}
