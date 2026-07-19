package rules

import (
	"strings"
	"testing"
)

func checkProbes(t *testing.T, doc string) []Finding {
	t.Helper()
	var findings []Finding
	for _, obj := range parseAll(t, doc) {
		findings = append(findings, ModelServerNoProbes{}.Check(obj)...)
	}
	return findings
}

func TestModelServerNoProbes(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		fixtureFile  string
		wantFindings int
	}{
		{
			name:         "bad fixture: vllm without probes and triton with liveness only",
			fixtureFile:  "model-server-no-probes/bad.yaml",
			wantFindings: 2,
		},
		{
			name:         "good fixture is clean",
			fixtureFile:  "model-server-no-probes/good.yaml",
			wantFindings: 0,
		},
		{
			name: "vllm deployment without readiness probe is flagged",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d, namespace: ml}
spec:
  template:
    spec:
      containers:
        - name: server
          image: vllm/vllm-openai:v0.5.0
`,
			wantFindings: 1,
		},
		{
			name: "image match is case-insensitive",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d}
spec:
  template:
    spec:
      containers:
        - name: server
          image: registry.example.com/ML/VLLM-Custom:latest
`,
			wantFindings: 1,
		},
		{
			name: "ollama statefulset without probes is flagged",
			doc: `
apiVersion: apps/v1
kind: StatefulSet
metadata: {name: s}
spec:
  template:
    spec:
      containers:
        - name: llm
          image: ollama/ollama:0.3.0
`,
			wantFindings: 1,
		},
		{
			name: "kserve predictor image is recognized",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d}
spec:
  template:
    spec:
      containers:
        - name: predictor
          image: kserve/huggingfaceserver:v0.13.0
`,
			wantFindings: 1,
		},
		{
			name: "non-model-server image is out of scope",
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
			name: "job with model server image is batch, not serving",
			doc: `
apiVersion: batch/v1
kind: Job
metadata: {name: j}
spec:
  template:
    spec:
      containers:
        - name: scorer
          image: vllm/vllm-openai:v0.5.0
`,
			wantFindings: 0,
		},
		{
			name: "init container is not a server",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d}
spec:
  template:
    spec:
      initContainers:
        - name: fetch-weights
          image: vllm/vllm-openai:v0.5.0
      containers:
        - name: web
          image: nginx:1.27
`,
			wantFindings: 0,
		},
		{
			name: "readiness probe present passes",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d}
spec:
  template:
    spec:
      containers:
        - name: server
          image: vllm/vllm-openai:v0.5.0
          readinessProbe:
            httpGet: {path: /health, port: 8000}
`,
			wantFindings: 0,
		},
		{
			name: "CRD kinds are left to their operators",
			doc: `
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata: {name: isvc}
spec:
  predictor:
    template:
      spec:
        containers:
          - name: kserve-container
            image: vllm/vllm-openai:v0.5.0
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
			findings := checkProbes(t, doc)
			if len(findings) != tt.wantFindings {
				t.Fatalf("got %d findings %v, want %d", len(findings), findings, tt.wantFindings)
			}
			for _, f := range findings {
				if f.RuleID != "model-server-no-probes" {
					t.Errorf("RuleID = %q, want %q", f.RuleID, "model-server-no-probes")
				}
				if f.Severity != Warning {
					t.Errorf("Severity = %v, want Warning", f.Severity)
				}
				if !strings.Contains(f.Message, "readinessProbe") {
					t.Errorf("Message = %q, want it to mention readinessProbe", f.Message)
				}
				if !strings.Contains(f.Fix, "startupProbe") {
					t.Errorf("Fix = %q, want it to recommend a startupProbe", f.Fix)
				}
				if !strings.HasSuffix(f.Detail, "/readinessProbe") {
					t.Errorf("Detail = %q, want container-name/readinessProbe", f.Detail)
				}
			}
		})
	}
}

func TestIsModelServerImage(t *testing.T) {
	for _, tt := range []struct {
		image string
		want  bool
	}{
		{"vllm/vllm-openai:v0.5.0", true},
		{"nvcr.io/nvidia/tritonserver:24.05-py3", true},
		{"ghcr.io/huggingface/text-generation-inference:2.0", true},
		{"pytorch/torchserve:0.11.0", true},
		{"lmsysorg/sglang:v0.2.0", true},
		{"ollama/ollama:0.3.0", true},
		{"kserve/sklearnserver:v0.13.0", true},
		{"kserve/xgbserver:latest", true},
		{"kserve/lgbserver:latest", true},
		{"kserve/pmmlserver:latest", true},
		{"kserve/paddleserver:latest", true},
		{"kserve/huggingfaceserver:latest-gpu", true},
		{"nvcr.io/nim/meta/llama3-8b-instruct:1.0", true},
		{"nginx:1.27", false},
		{"pytorch/pytorch:2.3.0-cuda12.1-cudnn8-runtime", false},
		{"rayproject/ray:2.32.0", false}, // generic Ray image: also used for training, deliberately not matched
		// KServe control plane, not model servers: the dogfood run caught a
		// bare "kserve/" pattern flagging the operator's own controller.
		{"kserve/kserve-controller:v0.19.0", false},
		{"kserve/storage-initializer:v0.19.0", false},
		{"kserve/kserve-agent:v0.19.0", false},
	} {
		got, _ := IsModelServerImage(tt.image)
		if got != tt.want {
			t.Errorf("IsModelServerImage(%q) = %v, want %v", tt.image, got, tt.want)
		}
	}
}
