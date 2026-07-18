package rules

import (
	"strings"
	"testing"
)

func checkGang(t *testing.T, doc string) []Finding {
	t.Helper()
	var findings []Finding
	for _, obj := range parseAll(t, doc) {
		findings = append(findings, DistributedTrainingNoGangScheduling{}.Check(obj)...)
	}
	return findings
}

const gangWorkerSpecs = `
  pytorchReplicaSpecs:
    Worker:
      replicas: 4
      template:
        spec:
          containers:
            - name: pytorch
              image: pytorch/pytorch:2.3.0-cuda12.1-cudnn8-runtime
              resources:
                limits: {nvidia.com/gpu: 1}
`

func TestDistributedTrainingNoGangScheduling(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		fixtureFile  string
		wantFindings int
		wantInMsg    string
	}{
		{
			name:         "bad fixture: pytorchjob and mpijob without gang evidence",
			fixtureFile:  "distributed-training-no-gang-scheduling/bad.yaml",
			wantFindings: 2,
		},
		{
			name:         "good fixture is clean",
			fixtureFile:  "distributed-training-no-gang-scheduling/good.yaml",
			wantFindings: 0,
		},
		{
			name:         "multi-replica gpu pytorchjob without gang evidence is flagged",
			doc:          "apiVersion: kubeflow.org/v1\nkind: PyTorchJob\nmetadata: {name: t, namespace: ml}\nspec:" + gangWorkerSpecs,
			wantFindings: 1,
			wantInMsg:    "4 replicas",
		},
		{
			name: "tfjob is covered too",
			doc: `
apiVersion: kubeflow.org/v1
kind: TFJob
metadata: {name: t}
spec:
  tfReplicaSpecs:
    Worker:
      replicas: 2
      template:
        spec:
          containers:
            - name: tensorflow
              image: tensorflow/tensorflow:2.16.1-gpu
              resources:
                limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 1,
		},
		{
			name: "roles without explicit replicas default to 1 each",
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
			wantInMsg:    "2 replicas",
		},
		{
			name:         "explicit default-scheduler is not gang evidence",
			doc:          "apiVersion: kubeflow.org/v1\nkind: PyTorchJob\nmetadata: {name: t}\nspec:\n  pytorchReplicaSpecs:\n    Worker:\n      replicas: 4\n      template:\n        spec:\n          schedulerName: default-scheduler\n          containers:\n            - name: pytorch\n              image: pytorch/pytorch:2.3.0\n              resources:\n                limits: {nvidia.com/gpu: 1}",
			wantFindings: 1,
		},
		{
			name:         "volcano schedulerName in the pod template passes",
			doc:          "apiVersion: kubeflow.org/v1\nkind: PyTorchJob\nmetadata: {name: t}\nspec:\n  pytorchReplicaSpecs:\n    Worker:\n      replicas: 4\n      template:\n        spec:\n          schedulerName: volcano\n          containers:\n            - name: pytorch\n              image: pytorch/pytorch:2.3.0\n              resources:\n                limits: {nvidia.com/gpu: 1}",
			wantFindings: 0,
		},
		{
			name:         "coscheduling pod-group label on the template passes",
			doc:          "apiVersion: kubeflow.org/v1\nkind: PyTorchJob\nmetadata: {name: t}\nspec:\n  pytorchReplicaSpecs:\n    Worker:\n      replicas: 4\n      template:\n        metadata:\n          labels: {scheduling.x-k8s.io/pod-group: pg1}\n        spec:\n          containers:\n            - name: pytorch\n              image: pytorch/pytorch:2.3.0\n              resources:\n                limits: {nvidia.com/gpu: 1}",
			wantFindings: 0,
		},
		{
			name:         "volcano group annotation on the object passes",
			doc:          "apiVersion: kubeflow.org/v1\nkind: PyTorchJob\nmetadata:\n  name: t\n  annotations: {scheduling.k8s.io/group-name: my-group}\nspec:" + gangWorkerSpecs,
			wantFindings: 0,
		},
		{
			name:         "kai queue label passes",
			doc:          "apiVersion: kubeflow.org/v1\nkind: PyTorchJob\nmetadata:\n  name: t\n  labels: {kai.scheduler/queue: research}\nspec:" + gangWorkerSpecs,
			wantFindings: 0,
		},
		{
			name: "plain deployment is out of scope",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d}
spec:
  replicas: 4
  template:
    spec:
      containers:
        - name: app
          image: ghcr.io/example/llm-api:1.0
          resources:
            limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc
			if tt.fixtureFile != "" {
				doc = readFixture(t, tt.fixtureFile)
			}
			findings := checkGang(t, doc)
			if len(findings) != tt.wantFindings {
				t.Fatalf("got %d findings %v, want %d", len(findings), findings, tt.wantFindings)
			}
			for _, f := range findings {
				if f.RuleID != "distributed-training-no-gang-scheduling" {
					t.Errorf("RuleID = %q, want %q", f.RuleID, "distributed-training-no-gang-scheduling")
				}
				if f.Severity != Warning {
					t.Errorf("Severity = %v, want Warning", f.Severity)
				}
				if !strings.Contains(f.Message, "gang") {
					t.Errorf("Message = %q, want it to mention gang scheduling", f.Message)
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
