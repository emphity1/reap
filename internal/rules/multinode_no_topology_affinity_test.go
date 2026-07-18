package rules

import (
	"strings"
	"testing"
)

func checkTopology(t *testing.T, doc string) []Finding {
	t.Helper()
	var findings []Finding
	for _, obj := range parseAll(t, doc) {
		findings = append(findings, MultinodeNoTopologyAffinity{}.Check(obj)...)
	}
	return findings
}

const topoJobHeader = "apiVersion: kubeflow.org/v1\nkind: PyTorchJob\nmetadata: {name: t, namespace: ml}\nspec:\n  runPolicy:\n    schedulingPolicy: {minAvailable: 4}\n"

func TestMultinodeNoTopologyAffinity(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		fixtureFile  string
		wantFindings int
	}{
		{
			name:         "bad fixture: gang-scheduled but topology-blind",
			fixtureFile:  "multinode-no-topology-affinity/bad.yaml",
			wantFindings: 1,
		},
		{
			name:         "good fixture is clean",
			fixtureFile:  "multinode-no-topology-affinity/good.yaml",
			wantFindings: 0,
		},
		{
			name: "multi-replica gpu job without placement is flagged",
			doc: topoJobHeader + `
  pytorchReplicaSpecs:
    Worker:
      replicas: 4
      template:
        spec:
          containers:
            - name: pytorch
              image: pytorch/pytorch:2.3.0
              resources:
                limits: {nvidia.com/gpu: 8}
`,
			wantFindings: 1,
		},
		{
			name: "pod affinity suppresses",
			doc: topoJobHeader + `
  pytorchReplicaSpecs:
    Worker:
      replicas: 4
      template:
        spec:
          affinity:
            podAffinity:
              preferredDuringSchedulingIgnoredDuringExecution:
                - weight: 100
                  podAffinityTerm:
                    topologyKey: topology.kubernetes.io/zone
                    labelSelector:
                      matchLabels: {job: t}
          containers:
            - name: pytorch
              image: pytorch/pytorch:2.3.0
              resources:
                limits: {nvidia.com/gpu: 8}
`,
			wantFindings: 0,
		},
		{
			name: "topologySpreadConstraints suppress",
			doc: topoJobHeader + `
  pytorchReplicaSpecs:
    Worker:
      replicas: 4
      template:
        spec:
          topologySpreadConstraints:
            - maxSkew: 1
              topologyKey: topology.kubernetes.io/zone
              whenUnsatisfiable: DoNotSchedule
              labelSelector:
                matchLabels: {job: t}
          containers:
            - name: pytorch
              image: pytorch/pytorch:2.3.0
              resources:
                limits: {nvidia.com/gpu: 8}
`,
			wantFindings: 0,
		},
		{
			name: "nodeSelector suppresses",
			doc: topoJobHeader + `
  pytorchReplicaSpecs:
    Worker:
      replicas: 4
      template:
        spec:
          nodeSelector: {node-group: gpu-compact}
          containers:
            - name: pytorch
              image: pytorch/pytorch:2.3.0
              resources:
                limits: {nvidia.com/gpu: 8}
`,
			wantFindings: 0,
		},
		{
			name: "kueue topology-aware-scheduling annotation suppresses",
			doc: topoJobHeader + `
  pytorchReplicaSpecs:
    Worker:
      replicas: 4
      template:
        metadata:
          annotations:
            kueue.x-k8s.io/podset-preferred-topology: cloud.provider.com/topology-block
        spec:
          containers:
            - name: pytorch
              image: pytorch/pytorch:2.3.0
              resources:
                limits: {nvidia.com/gpu: 8}
`,
			wantFindings: 0,
		},
		{
			name: "single replica is out of scope",
			doc: topoJobHeader + `
  pytorchReplicaSpecs:
    Worker:
      replicas: 1
      template:
        spec:
          containers:
            - name: pytorch
              image: pytorch/pytorch:2.3.0
              resources:
                limits: {nvidia.com/gpu: 8}
`,
			wantFindings: 0,
		},
		{
			name: "cpu-only distributed job is out of scope",
			doc: topoJobHeader + `
  pytorchReplicaSpecs:
    Worker:
      replicas: 4
      template:
        spec:
          containers:
            - name: pytorch
              image: pytorch/pytorch:2.3.0
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
			findings := checkTopology(t, doc)
			if len(findings) != tt.wantFindings {
				t.Fatalf("got %d findings %v, want %d", len(findings), findings, tt.wantFindings)
			}
			for _, f := range findings {
				if f.RuleID != "multinode-no-topology-affinity" {
					t.Errorf("RuleID = %q, want %q", f.RuleID, "multinode-no-topology-affinity")
				}
				if f.Severity != Info {
					t.Errorf("Severity = %v, want Info", f.Severity)
				}
				if !strings.Contains(f.Message, "NCCL") {
					t.Errorf("Message = %q, want it to explain the NCCL cost", f.Message)
				}
			}
		})
	}
}
