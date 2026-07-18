package rules

import (
	"fmt"

	"github.com/emphity1/reap/internal/parser"
)

// NoGPULimit flags containers that request a GPU without an equal limit.
// Kubernetes treats GPUs as extended resources: a request without a limit is
// rejected at admission, and when both are set they must be equal. Catching
// this in CI means the manifest fails review instead of failing at rollout,
// while the GPU it was meant to reserve sits idle.
type NoGPULimit struct{}

func (NoGPULimit) ID() string { return "no-gpu-limit" }

func (NoGPULimit) Severity() Severity { return Error }

func (r NoGPULimit) Check(obj parser.Object) []Finding {
	var findings []Finding
	for _, c := range obj.Containers() {
		for _, res := range gpuResources(c) {
			req, hasReq := c.Requests[res]
			lim, hasLim := c.Limits[res]
			var msg string
			switch {
			case hasReq && !hasLim:
				msg = fmt.Sprintf("container %q requests %s: %s but sets no limit; "+
					"Kubernetes rejects GPU requests without an equal limit, so this manifest will not deploy", c.Name, res, req)
			case hasReq && hasLim && req != lim:
				msg = fmt.Sprintf("container %q requests %s: %s but limits it to %s; "+
					"GPU request and limit must be equal", c.Name, res, req, lim)
			default:
				continue
			}
			findings = append(findings, Finding{
				RuleID:    r.ID(),
				Severity:  r.Severity(),
				Message:   msg,
				ObjectRef: obj.Ref(),
				Source:    obj.Source,
				Fix:       fmt.Sprintf("set resources.limits[%q] equal to the request", res),
			})
		}
	}
	return findings
}
