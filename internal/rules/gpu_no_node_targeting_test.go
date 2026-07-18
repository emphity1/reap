package rules

import (
	"strings"
	"testing"
)

func checkNodeTargeting(t *testing.T, doc string) []Finding {
	t.Helper()
	var findings []Finding
	for _, obj := range parseAll(t, doc) {
		findings = append(findings, GPUNoNodeTargeting{}.Check(obj)...)
	}
	return findings
}

func TestGPUNoNodeTargeting(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		fixtureFile  string
		wantFindings int
	}{
		{
			name:         "bad fixture: two GPU workloads with no node targeting",
			fixtureFile:  "gpu-no-node-targeting/bad.yaml",
			wantFindings: 2,
		},
		{
			name:         "good fixture is clean",
			fixtureFile:  "gpu-no-node-targeting/good.yaml",
			wantFindings: 0,
		},
		{
			name: "gpu pod without any targeting is flagged",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p, namespace: ml}
spec:
  containers:
    - name: c
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 1,
		},
		{
			name: "toleration suppresses",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  tolerations:
    - key: nvidia.com/gpu
      operator: Exists
      effect: NoSchedule
  containers:
    - name: c
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "nodeSelector suppresses",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  nodeSelector: {nvidia.com/gpu.present: "true"}
  containers:
    - name: c
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "node affinity suppresses",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: gpu-type
                operator: In
                values: [a100]
  containers:
    - name: c
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "pod anti-affinity alone does not target nodes",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - topologyKey: kubernetes.io/hostname
          labelSelector:
            matchLabels: {app: p}
  containers:
    - name: c
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 1,
		},
		{
			name: "empty tolerations list is not targeting",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  tolerations: []
  containers:
    - name: c
      resources:
        limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 1,
		},
		{
			name: "cpu-only workload is out of scope",
			doc: `
apiVersion: v1
kind: Pod
metadata: {name: p}
spec:
  containers:
    - name: c
      resources:
        limits: {cpu: "4"}
`,
			wantFindings: 0,
		},
		{
			name: "pytorchjob with untargeted gpu workers is flagged once",
			doc: `
apiVersion: kubeflow.org/v1
kind: PyTorchJob
metadata: {name: t}
spec:
  pytorchReplicaSpecs:
    Master:
      template:
        spec:
          containers:
            - name: pytorch
              image: pytorch/pytorch:2.3.0
              resources:
                limits: {nvidia.com/gpu: 1}
    Worker:
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc
			if tt.fixtureFile != "" {
				doc = readFixture(t, tt.fixtureFile)
			}
			findings := checkNodeTargeting(t, doc)
			if len(findings) != tt.wantFindings {
				t.Fatalf("got %d findings %v, want %d", len(findings), findings, tt.wantFindings)
			}
			for _, f := range findings {
				if f.RuleID != "gpu-no-node-targeting" {
					t.Errorf("RuleID = %q, want %q", f.RuleID, "gpu-no-node-targeting")
				}
				if f.Severity != Info {
					t.Errorf("Severity = %v, want Info", f.Severity)
				}
				if !strings.Contains(f.Message, "Pending") {
					t.Errorf("Message = %q, want it to mention the Pending risk", f.Message)
				}
				if f.ObjectRef == "" || f.Source == "" || f.Fix == "" {
					t.Errorf("finding has empty ObjectRef/Source/Fix: %+v", f)
				}
			}
		})
	}
}
