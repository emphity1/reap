package rules

import (
	"strings"
	"testing"
)

func checkCulling(t *testing.T, doc string) []Finding {
	t.Helper()
	var findings []Finding
	for _, obj := range parseAll(t, doc) {
		findings = append(findings, NotebookNoIdleCulling{}.Check(obj)...)
	}
	return findings
}

func TestNotebookNoIdleCulling(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		fixtureFile  string
		wantFindings int
	}{
		{
			name:         "bad fixture: jupyter statefulset and kubeflow Notebook CRD",
			fixtureFile:  "notebook-no-idle-culling/bad.yaml",
			wantFindings: 2,
		},
		{
			name:         "good fixture is clean",
			fixtureFile:  "notebook-no-idle-culling/good.yaml",
			wantFindings: 0,
		},
		{
			name: "gpu jupyter pod without culling is flagged",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: nb, namespace: ml}
spec:
  containers:
    - name: notebook
      image: jupyter/tensorflow-notebook:latest
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 1,
		},
		{
			name: "culling arg suppresses",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: nb}
spec:
  containers:
    - name: notebook
      image: jupyter/tensorflow-notebook:latest
      args: ["--MappingKernelManager.cull_idle_timeout=3600"]
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "shutdown_no_activity in command suppresses",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: nb}
spec:
  containers:
    - name: notebook
      image: jupyter/tensorflow-notebook:latest
      command: ["jupyter", "lab", "--NotebookApp.shutdown_no_activity_timeout=1800"]
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "culling env var suppresses",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: nb}
spec:
  containers:
    - name: notebook
      image: jupyter/tensorflow-notebook:latest
      env:
        - name: JUPYTER_CULL_IDLE_TIMEOUT
          value: "3600"
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "cpu-only notebook is out of scope",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: nb}
spec:
  containers:
    - name: notebook
      image: jupyter/base-notebook:latest
`,
			wantFindings: 0,
		},
		{
			name: "gpu workload with a non-notebook image is out of scope",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  containers:
    - name: c
      image: pytorch/pytorch:2.3.0
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "kind Notebook counts even with a custom image",
			doc: `
apiVersion: kubeflow.org/v1
kind: Notebook
metadata: {name: nb, namespace: ml}
spec:
  template:
    spec:
      containers:
        - name: custom
          image: ghcr.io/example/research-env:2.1
          resources:
            limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc
			if tt.fixtureFile != "" {
				doc = readFixture(t, tt.fixtureFile)
			}
			findings := checkCulling(t, doc)
			if len(findings) != tt.wantFindings {
				t.Fatalf("got %d findings %v, want %d", len(findings), findings, tt.wantFindings)
			}
			for _, f := range findings {
				if f.RuleID != "notebook-no-idle-culling" {
					t.Errorf("RuleID = %q, want %q", f.RuleID, "notebook-no-idle-culling")
				}
				if f.Severity != Info {
					t.Errorf("Severity = %v, want Info", f.Severity)
				}
				if !strings.HasSuffix(f.Detail, "/idle-culling") {
					t.Errorf("Detail = %q, want container-name/idle-culling", f.Detail)
				}
				if !strings.Contains(f.Message, "idle") {
					t.Errorf("Message = %q, want it to mention idle GPU cost", f.Message)
				}
			}
		})
	}
}
