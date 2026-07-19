package rules

import (
	"strings"
	"testing"
)

func checkShm(t *testing.T, doc string) []Finding {
	t.Helper()
	var findings []Finding
	for _, obj := range parseAll(t, doc) {
		findings = append(findings, ShmTooSmall{}.Check(obj)...)
	}
	return findings
}

func TestShmTooSmall(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		fixtureFile  string
		wantFindings int
	}{
		{
			name:         "bad fixture: no shm mount and disk-backed emptyDir",
			fixtureFile:  "shm-too-small/bad.yaml",
			wantFindings: 2,
		},
		{
			name:         "good fixture is clean",
			fixtureFile:  "shm-too-small/good.yaml",
			wantFindings: 0,
		},
		{
			name: "gpu container without shm mount is flagged",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d, namespace: ml}
spec:
  template:
    spec:
      containers:
        - name: worker
          image: pytorch/pytorch:2.3.0
          resources:
            limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 1,
		},
		{
			name: "memory-backed shm mount passes",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d}
spec:
  template:
    spec:
      volumes:
        - name: shm
          emptyDir: {medium: Memory}
      containers:
        - name: worker
          image: pytorch/pytorch:2.3.0
          volumeMounts:
            - name: shm
              mountPath: /dev/shm
          resources:
            limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "disk-backed emptyDir at /dev/shm is still flagged",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d}
spec:
  template:
    spec:
      volumes:
        - name: shm
          emptyDir: {}
      containers:
        - name: worker
          image: pytorch/pytorch:2.3.0
          volumeMounts:
            - name: shm
              mountPath: /dev/shm
          resources:
            limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 1,
		},
		{
			name: "cpu-only workload is out of scope",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d}
spec:
  template:
    spec:
      containers:
        - name: web
          image: nginx:1.27
`,
			wantFindings: 0,
		},
		{
			name: "hostIPC shares the host shm and passes",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  hostIPC: true
  containers:
    - name: worker
      image: pytorch/pytorch:2.3.0
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "pytorchjob CRD is covered via the pod-spec fallback",
			doc: `
apiVersion: kubeflow.org/v1
kind: PyTorchJob
metadata: {name: t}
spec:
  pytorchReplicaSpecs:
    Worker:
      replicas: 2
      template:
        spec:
          containers:
            - name: pytorch
              image: pytorch/pytorch:2.3.0
              resources:
                limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 1,
		},
		{
			name: "only the gpu container in a mixed pod is flagged",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d}
spec:
  template:
    spec:
      containers:
        - name: worker
          image: pytorch/pytorch:2.3.0
          resources:
            limits: {nvidia.com/gpu: 1}
        - name: sidecar
          image: envoyproxy/envoy:v1.30
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
			findings := checkShm(t, doc)
			if len(findings) != tt.wantFindings {
				t.Fatalf("got %d findings %v, want %d", len(findings), findings, tt.wantFindings)
			}
			for _, f := range findings {
				if f.RuleID != "shm-too-small" {
					t.Errorf("RuleID = %q, want %q", f.RuleID, "shm-too-small")
				}
				if f.Severity != Warning {
					t.Errorf("Severity = %v, want Warning", f.Severity)
				}
				if !strings.Contains(f.Message, "/dev/shm") {
					t.Errorf("Message = %q, want it to mention /dev/shm", f.Message)
				}
				if !strings.Contains(f.Fix, "medium: Memory") {
					t.Errorf("Fix = %q, want it to mention medium: Memory", f.Fix)
				}
				// The rule is deliberately broad; the escape hatch for
				// runtimes that never use shared memory must stay in the fix.
				if !strings.Contains(f.Fix, "baseline") {
					t.Errorf("Fix = %q, want it to name the baseline escape hatch", f.Fix)
				}
				if !strings.HasSuffix(f.Detail, "/dev-shm") {
					t.Errorf("Detail = %q, want container-name/dev-shm", f.Detail)
				}
			}
		})
	}
}
