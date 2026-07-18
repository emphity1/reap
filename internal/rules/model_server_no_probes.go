package rules

import (
	"fmt"

	"github.com/emphity1/reap/internal/parser"
)

// servingKinds are the workload kinds that receive Service traffic. Jobs and
// CronJobs are batch — no traffic, no probes needed. CRD-managed serving
// (KServe InferenceService, RayService) is excluded on purpose: their
// operators own probe configuration, and flagging them would be false
// positives.
var servingKinds = map[string]bool{
	"Pod":         true,
	"Deployment":  true,
	"ReplicaSet":  true,
	"StatefulSet": true,
	"DaemonSet":   true,
}

// ModelServerNoProbes flags recognized inference servers that define no
// readinessProbe: Kubernetes routes traffic the moment the container starts,
// which for LLM servers means minutes of failed requests while weights load.
// The fix deliberately recommends a startupProbe + readinessProbe pair, not
// an aggressive livenessProbe — a naive liveness probe kills slow-starting
// servers into a restart loop.
type ModelServerNoProbes struct{}

func (ModelServerNoProbes) ID() string { return "model-server-no-probes" }

func (ModelServerNoProbes) Severity() Severity { return Warning }

func (r ModelServerNoProbes) Check(obj parser.Object) []Finding {
	if !servingKinds[obj.Kind] {
		return nil
	}
	var findings []Finding
	for _, c := range obj.Containers() {
		if c.Init {
			continue
		}
		isServer, pattern := IsModelServerImage(c.Image)
		if !isServer {
			continue
		}
		if _, ok := c.Raw["readinessProbe"]; ok {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   r.ID(),
			Severity: r.Severity(),
			Message: fmt.Sprintf("container %q runs a model server (%s) with no readinessProbe; "+
				"Kubernetes routes traffic as soon as the container starts — for LLM servers that is minutes of failed requests while weights load", c.Name, pattern),
			ObjectRef: obj.Ref(),
			Detail:    c.Name + "/readinessProbe",
			Source:    obj.Source,
			Fix: "add a readinessProbe on the serving endpoint plus a startupProbe with a generous failureThreshold " +
				"(weight loading can take minutes); avoid an aggressive livenessProbe — it kills slow-starting servers into a restart loop",
		})
	}
	return findings
}
