package rules

import (
	"fmt"

	"github.com/emphity1/reap/internal/parser"
)

// GPUNoNodeTargeting flags GPU workloads whose pod spec carries no
// nodeSelector, node affinity, or tolerations. GPU node pools are commonly
// tainted; a pod that does not tolerate the taint sits Pending forever, and
// the cluster autoscaler will not scale a tainted GPU pool up for it.
//
// Info severity on purpose: any of the three fields suppresses (we cannot
// statically judge whether a toleration matches the cluster's actual taint),
// and clusters running the ExtendedResourceToleration admission controller
// add the toleration automatically, making the manifest legitimately bare.
type GPUNoNodeTargeting struct{}

func (GPUNoNodeTargeting) ID() string { return "gpu-no-node-targeting" }

func (GPUNoNodeTargeting) Severity() Severity { return Info }

func (r GPUNoNodeTargeting) Check(obj parser.Object) []Finding {
	flagged := false
	for _, ps := range obj.PodSpecs() {
		hasGPU := false
		for _, c := range ps.Containers {
			if len(gpuResources(c)) > 0 {
				hasGPU = true
				break
			}
		}
		if hasGPU && !hasNodeTargeting(ps) {
			flagged = true
			break
		}
	}
	if !flagged {
		return nil
	}
	return []Finding{{
		RuleID:   r.ID(),
		Severity: r.Severity(),
		Message: fmt.Sprintf("%s %q requests GPUs but sets no nodeSelector, node affinity, or tolerations; "+
			"on a cluster with tainted GPU nodes it sits Pending indefinitely, and the autoscaler will not scale a tainted GPU pool up for it",
			obj.Kind, obj.Name),
		ObjectRef: obj.Ref(),
		Source:    obj.Source,
		Fix: "add a toleration for your GPU node taint (e.g. key nvidia.com/gpu, operator Exists) and/or a nodeSelector for GPU nodes; " +
			"if your cluster runs the ExtendedResourceToleration admission controller, baseline this finding",
	}}
}

// hasNodeTargeting reports whether the pod spec expresses any node
// placement intent. Any non-empty nodeSelector or tolerations, or any
// nodeAffinity, counts — suppression errs on acceptance.
func hasNodeTargeting(ps parser.PodSpec) bool {
	if sel, ok := ps.Raw["nodeSelector"].(map[string]any); ok && len(sel) > 0 {
		return true
	}
	if tol, ok := ps.Raw["tolerations"].([]any); ok && len(tol) > 0 {
		return true
	}
	if aff, ok := ps.Raw["affinity"].(map[string]any); ok {
		if _, ok := aff["nodeAffinity"]; ok {
			return true
		}
	}
	return false
}
