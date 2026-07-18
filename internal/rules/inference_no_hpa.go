package rules

import (
	"fmt"

	"github.com/emphity1/reap/internal/parser"
)

// scalableServingKinds are the workload kinds an HPA can target and that
// serve inference traffic.
var scalableServingKinds = map[string]bool{
	"Deployment":  true,
	"StatefulSet": true,
}

// InferenceNoHPA flags model-server workloads with no HorizontalPodAutoscaler
// targeting them in the linted input. Fixed replica counts either idle GPUs
// off-peak or throttle traffic at peak. Reuses the model-server image
// detection shared with model-server-no-probes.
type InferenceNoHPA struct{}

func (InferenceNoHPA) ID() string { return "inference-no-hpa" }

func (InferenceNoHPA) Severity() Severity { return Info }

func (r InferenceNoHPA) CheckAll(objs []parser.Object, idx *Index) []Finding {
	var findings []Finding
	for _, obj := range objs {
		pattern, ok := servedModelServer(obj)
		if !ok {
			continue
		}
		if len(idx.HPAsTargeting(obj.Kind, obj.Namespace, obj.Name)) > 0 {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   r.ID(),
			Severity: r.Severity(),
			Message: fmt.Sprintf("model server %s %q (%s) has no HorizontalPodAutoscaler targeting it in the linted input; "+
				"a fixed replica count either idles GPUs off-peak or throttles traffic at peak", obj.Kind, obj.Name, pattern),
			ObjectRef: obj.Ref(),
			Source:    obj.Source,
			Fix: "add an HPA for it (LLM servers scale better on custom metrics like queue depth or GPU utilization than on CPU); " +
				"if the HPA lives outside these manifests, baseline this finding",
		})
	}
	return findings
}

// servedModelServer reports whether the object is a scalable serving
// workload running a recognized model-server image, and which image pattern
// matched.
func servedModelServer(obj parser.Object) (string, bool) {
	if !scalableServingKinds[obj.Kind] {
		return "", false
	}
	for _, c := range obj.Containers() {
		if c.Init {
			continue
		}
		if ok, pattern := IsModelServerImage(c.Image); ok {
			return pattern, true
		}
	}
	return "", false
}
