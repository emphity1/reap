package rules

import (
	"fmt"
	"strings"

	"github.com/emphity1/reap/internal/parser"
)

// cullingMarkers are substrings that indicate idle-culling configuration in
// container args, command, or env var names. Suppression only.
var cullingMarkers = []string{"cull", "shutdown_no_activity"}

// NotebookNoIdleCulling flags GPU notebook workloads with no visible
// idle-culling configuration: a notebook forgotten on Friday holds its GPU
// all weekend, and an idle A100 costs the same as a busy one.
//
// Genuinely heuristic, hence info: culling often lives platform-side
// (JupyterHub Helm values, the Kubeflow notebook controller) where a
// manifest linter cannot see it — expect false negatives there, and
// baseline the finding when the platform culls for you.
type NotebookNoIdleCulling struct{}

func (NotebookNoIdleCulling) ID() string { return "notebook-no-idle-culling" }

func (NotebookNoIdleCulling) Severity() Severity { return Info }

func (r NotebookNoIdleCulling) Check(obj parser.Object) []Finding {
	var findings []Finding
	for _, c := range obj.Containers() {
		if c.Init {
			continue
		}
		if !isNotebook(obj, c) {
			continue
		}
		gpus := gpuSummary([]parser.Container{c})
		if gpus == "" {
			continue
		}
		if hasCullingConfig(c) {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   r.ID(),
			Severity: r.Severity(),
			Message: fmt.Sprintf("notebook container %q holds %s with no idle-culling configuration visible; "+
				"a notebook forgotten on Friday keeps its GPU allocated all weekend — idle accelerators cost the same as busy ones", c.Name, gpus),
			ObjectRef: obj.Ref(),
			Detail:    c.Name + "/idle-culling",
			Source:    obj.Source,
			Fix: "enable idle culling (e.g. --MappingKernelManager.cull_idle_timeout=3600 or the JupyterHub idle culler); " +
				"if culling is enforced platform-side (Hub config, notebook controller), baseline this finding",
		})
	}
	return findings
}

// isNotebook detects notebook workloads: the Kubeflow Notebook kind (any
// image), or an image whose name mentions jupyter.
func isNotebook(obj parser.Object, c parser.Container) bool {
	if obj.Kind == "Notebook" {
		return true
	}
	return strings.Contains(strings.ToLower(c.Image), "jupyter")
}

// hasCullingConfig scans args, command, and env var names for culling
// markers. Suppression errs on acceptance.
func hasCullingConfig(c parser.Container) bool {
	for _, key := range []string{"args", "command"} {
		items, _ := c.Raw[key].([]any)
		for _, item := range items {
			if s, ok := item.(string); ok && containsCullingMarker(s) {
				return true
			}
		}
	}
	env, _ := c.Raw["env"].([]any)
	for _, item := range env {
		if m, ok := item.(map[string]any); ok {
			if name, ok := m["name"].(string); ok && containsCullingMarker(name) {
				return true
			}
		}
	}
	return false
}

func containsCullingMarker(s string) bool {
	lower := strings.ToLower(s)
	for _, marker := range cullingMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
