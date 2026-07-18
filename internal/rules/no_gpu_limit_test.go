package rules

import (
	"strings"
	"testing"
)

// checkYAML runs the NoGPULimit rule over every object in the YAML document.
func checkYAML(t *testing.T, doc string) []Finding {
	t.Helper()
	var findings []Finding
	for _, obj := range parseAll(t, doc) {
		findings = append(findings, NoGPULimit{}.Check(obj)...)
	}
	return findings
}

func TestNoGPULimit(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		wantFindings int
		wantInMsg    string // substring expected in the first finding's message
	}{
		{
			name:         "bad fixture: request without limit and request/limit mismatch",
			doc:          "", // loaded from testdata in the test body
			wantFindings: 2,
		},
		{
			name:         "good fixture is clean",
			doc:          "",
			wantFindings: 0,
		},
		{
			name: "request equal to limit is clean",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  containers:
    - name: c
      resources:
        requests: {nvidia.com/gpu: 1}
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "limit without request is clean",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  containers:
    - name: c
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "request without limit is flagged",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  containers:
    - name: c
      resources:
        requests: {nvidia.com/gpu: 1}
`,
			wantFindings: 1,
			wantInMsg:    "no limit",
		},
		{
			name: "request different from limit is flagged",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  containers:
    - name: c
      resources:
        requests: {nvidia.com/gpu: 1}
        limits: {nvidia.com/gpu: 2}
`,
			wantFindings: 1,
			wantInMsg:    "must be equal",
		},
		{
			name: "init container is checked too",
			doc: `
apiVersion: batch/v1
kind: Job
metadata: {name: j}
spec:
  template:
    spec:
      initContainers:
        - name: warmup
          resources:
            requests: {nvidia.com/gpu: 1}
      containers:
        - name: main
`,
			wantFindings: 1,
		},
		{
			name: "amd gpu resource is recognized",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  containers:
    - name: c
      resources:
        requests: {amd.com/gpu: 1}
`,
			wantFindings: 1,
		},
		{
			name: "nvidia MIG slice is recognized",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  containers:
    - name: c
      resources:
        requests: {nvidia.com/mig-1g.5gb: 1}
`,
			wantFindings: 1,
		},
		{
			name: "pytorchjob CRD containers are checked",
			doc: `
apiVersion: kubeflow.org/v1
kind: PyTorchJob
metadata: {name: train}
spec:
  pytorchReplicaSpecs:
    Worker:
      replicas: 3
      template:
        spec:
          containers:
            - name: worker
              resources:
                requests: {nvidia.com/gpu: 8}
`,
			wantFindings: 1,
		},
		{
			name: "cpu and memory only is out of scope",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  containers:
    - name: c
      resources:
        requests: {cpu: "4", memory: 16Gi}
`,
			wantFindings: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc
			switch tt.name {
			case "bad fixture: request without limit and request/limit mismatch":
				doc = readFixture(t, "no-gpu-limit/bad.yaml")
			case "good fixture is clean":
				doc = readFixture(t, "no-gpu-limit/good.yaml")
			}
			findings := checkYAML(t, doc)
			if len(findings) != tt.wantFindings {
				t.Fatalf("got %d findings %v, want %d", len(findings), findings, tt.wantFindings)
			}
			if len(findings) == 0 {
				return
			}
			for _, f := range findings {
				if f.RuleID != "no-gpu-limit" {
					t.Errorf("RuleID = %q, want %q", f.RuleID, "no-gpu-limit")
				}
				if f.Severity != Error {
					t.Errorf("Severity = %v, want Error", f.Severity)
				}
				if f.ObjectRef == "" || f.Source == "" || f.Fix == "" {
					t.Errorf("finding has empty ObjectRef/Source/Fix: %+v", f)
				}
			}
			if tt.wantInMsg != "" && !strings.Contains(findings[0].Message, tt.wantInMsg) {
				t.Errorf("Message = %q, want it to contain %q", findings[0].Message, tt.wantInMsg)
			}
		})
	}
}

func TestParseSeverity(t *testing.T) {
	for _, tt := range []struct {
		in      string
		want    Severity
		wantErr bool
	}{
		{in: "info", want: Info},
		{in: "warning", want: Warning},
		{in: "error", want: Error},
		{in: "bogus", wantErr: true},
	} {
		got, err := ParseSeverity(tt.in)
		if tt.wantErr != (err != nil) {
			t.Errorf("ParseSeverity(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
		}
		if err == nil && got != tt.want {
			t.Errorf("ParseSeverity(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
