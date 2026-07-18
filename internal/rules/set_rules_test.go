package rules

import (
	"strings"
	"testing"
)

func checkSet(t *testing.T, rule SetRule, doc string) []Finding {
	t.Helper()
	objs := parseAll(t, doc)
	return rule.CheckAll(objs, BuildIndex(objs))
}

const vllmDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata: {name: chat-api, namespace: ml}
spec:
  template:
    metadata:
      labels: {app: chat-api}
    spec:
      containers:
        - name: server
          image: vllm/vllm-openai:v0.5.0
          readinessProbe:
            httpGet: {path: /health, port: 8000}
          resources:
            limits: {nvidia.com/gpu: 1}
`

const chatAPIHPA = `
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata: {name: chat-api, namespace: ml}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: chat-api
  minReplicas: 1
  maxReplicas: 8
`

const chatAPIPDB = `
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: {name: chat-api, namespace: ml}
spec:
  minAvailable: 1
  selector:
    matchLabels: {app: chat-api}
`

func TestInferenceNoHPA(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		wantFindings int
	}{
		{"model server with no hpa in set", vllmDeployment, 1},
		{"hpa targeting it suppresses", vllmDeployment + "---" + chatAPIHPA, 0},
		{
			name: "hpa for a different workload does not count",
			doc: vllmDeployment + `
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata: {name: other, namespace: ml}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: other-api
`,
			wantFindings: 1,
		},
		{
			name: "hpa in another namespace does not count",
			doc: vllmDeployment + `
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata: {name: chat-api, namespace: staging}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: chat-api
`,
			wantFindings: 1,
		},
		{
			name: "non-model-server deployment is out of scope",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: ml}
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
			name: "job with a model server image is batch, not serving",
			doc: `
apiVersion: batch/v1
kind: Job
metadata: {name: score, namespace: ml}
spec:
  template:
    spec:
      containers:
        - name: scorer
          image: vllm/vllm-openai:v0.5.0
`,
			wantFindings: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := checkSet(t, InferenceNoHPA{}, tt.doc)
			if len(findings) != tt.wantFindings {
				t.Fatalf("got %d findings %v, want %d", len(findings), findings, tt.wantFindings)
			}
			for _, f := range findings {
				if f.RuleID != "inference-no-hpa" || f.Severity != Info {
					t.Errorf("finding = %+v, want rule inference-no-hpa at info", f)
				}
				if !strings.Contains(f.Message, "linted input") {
					t.Errorf("Message = %q, want it to say the judgment is scoped to the linted input", f.Message)
				}
			}
		})
	}
}

func TestNoPDB(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		wantFindings int
	}{
		{"model server with no pdb in set", vllmDeployment, 1},
		{"matching pdb suppresses", vllmDeployment + "---" + chatAPIPDB, 0},
		{
			name: "pdb with non-matching labels does not count",
			doc: vllmDeployment + `
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: {name: other, namespace: ml}
spec:
  minAvailable: 1
  selector:
    matchLabels: {app: other-api}
`,
			wantFindings: 1,
		},
		{
			name: "pdb in another namespace does not count",
			doc: vllmDeployment + `
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: {name: chat-api, namespace: staging}
spec:
  minAvailable: 1
  selector:
    matchLabels: {app: chat-api}
`,
			wantFindings: 1,
		},
		{
			name: "pdb using matchExpressions is assumed to match",
			doc: vllmDeployment + `
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: {name: expr, namespace: ml}
spec:
  minAvailable: 1
  selector:
    matchExpressions:
      - key: app
        operator: In
        values: [chat-api]
`,
			wantFindings: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := checkSet(t, NoPDB{}, tt.doc)
			if len(findings) != tt.wantFindings {
				t.Fatalf("got %d findings %v, want %d", len(findings), findings, tt.wantFindings)
			}
			for _, f := range findings {
				if f.RuleID != "no-pdb" || f.Severity != Info {
					t.Errorf("finding = %+v, want rule no-pdb at info", f)
				}
			}
		})
	}
}

func TestAllSetRegistersBothRules(t *testing.T) {
	ids := map[string]bool{}
	for _, r := range AllSet() {
		ids[r.ID()] = true
	}
	if !ids["inference-no-hpa"] || !ids["no-pdb"] {
		t.Errorf("AllSet() = %v, want inference-no-hpa and no-pdb", ids)
	}
}

func TestSetRuleFixtures(t *testing.T) {
	for _, tt := range []struct {
		rule    SetRule
		fixture string
		want    int
	}{
		{InferenceNoHPA{}, "inference-no-hpa/bad.yaml", 1},
		{InferenceNoHPA{}, "inference-no-hpa/good.yaml", 0},
		{NoPDB{}, "no-pdb/bad.yaml", 1},
		{NoPDB{}, "no-pdb/good.yaml", 0},
	} {
		findings := checkSet(t, tt.rule, readFixture(t, tt.fixture))
		if len(findings) != tt.want {
			t.Errorf("%s on %s: got %d findings %v, want %d",
				tt.rule.ID(), tt.fixture, len(findings), findings, tt.want)
		}
	}
}
