package parser

import (
	"strings"
	"testing"
)

func parse(t *testing.T, doc string) []Object {
	t.Helper()
	objs, err := Parse(strings.NewReader(doc), "test.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return objs
}

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want []string // expected Ref() of each object, in order
	}{
		{
			name: "single deployment",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: prod
`,
			want: []string{"Deployment/prod/web"},
		},
		{
			name: "namespace omitted from ref when empty",
			doc: `
apiVersion: v1
kind: Pod
metadata:
  name: solo
`,
			want: []string{"Pod/solo"},
		},
		{
			name: "multi-document stream with empty and comment-only docs",
			doc: `
apiVersion: v1
kind: Service
metadata:
  name: svc
  namespace: a
---
# Source: chart/templates/empty.yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
  namespace: a
`,
			want: []string{"Service/a/svc", "ConfigMap/a/cm"},
		},
		{
			name: "v1 List is flattened",
			doc: `
apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: Pod
    metadata:
      name: one
  - apiVersion: v1
    kind: Pod
    metadata:
      name: two
`,
			want: []string{"Pod/one", "Pod/two"},
		},
		{
			name: "document without kind is skipped",
			doc: `
foo: bar
`,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := parse(t, tt.doc)
			if len(objs) != len(tt.want) {
				t.Fatalf("got %d objects, want %d", len(objs), len(tt.want))
			}
			for i, want := range tt.want {
				if got := objs[i].Ref(); got != want {
					t.Errorf("object %d: Ref() = %q, want %q", i, got, want)
				}
				if objs[i].Source != "test.yaml" {
					t.Errorf("object %d: Source = %q, want %q", i, objs[i].Source, "test.yaml")
				}
			}
		})
	}
}

func TestParseInvalidYAML(t *testing.T) {
	_, err := Parse(strings.NewReader("kind: [unclosed"), "bad.yaml")
	if err == nil {
		t.Fatal("Parse accepted invalid YAML, want error")
	}
}

func TestContainers(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want []string // expected container names, in order
	}{
		{
			name: "pod",
			doc: `
apiVersion: v1
kind: Pod
metadata:
  name: p
spec:
  containers:
    - name: main
`,
			want: []string{"main"},
		},
		{
			name: "deployment with init container",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: d
spec:
  template:
    spec:
      initContainers:
        - name: setup
      containers:
        - name: app
        - name: sidecar
`,
			want: []string{"app", "sidecar", "setup"},
		},
		{
			name: "cronjob nested pod spec",
			doc: `
apiVersion: batch/v1
kind: CronJob
metadata:
  name: c
spec:
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: task
`,
			want: []string{"task"},
		},
		{
			name: "pytorchjob CRD found via template fallback",
			doc: `
apiVersion: kubeflow.org/v1
kind: PyTorchJob
metadata:
  name: train
spec:
  pytorchReplicaSpecs:
    Master:
      replicas: 1
      template:
        spec:
          containers:
            - name: master
    Worker:
      replicas: 3
      template:
        spec:
          containers:
            - name: worker
`,
			want: []string{"master", "worker"},
		},
		{
			name: "service has no containers",
			doc: `
apiVersion: v1
kind: Service
metadata:
  name: s
spec:
  ports:
    - port: 80
`,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := parse(t, tt.doc)
			if len(objs) != 1 {
				t.Fatalf("got %d objects, want 1", len(objs))
			}
			containers := objs[0].Containers()
			var names []string
			for _, c := range containers {
				names = append(names, c.Name)
			}
			if len(names) != len(tt.want) {
				t.Fatalf("container names = %v, want %v", names, tt.want)
			}
			for i := range tt.want {
				if names[i] != tt.want[i] {
					t.Fatalf("container names = %v, want %v", names, tt.want)
				}
			}
		})
	}
}

func TestContainerResourcesNormalized(t *testing.T) {
	objs := parse(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: d
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              nvidia.com/gpu: 1
              memory: 16Gi
            limits:
              nvidia.com/gpu: "2"
`)
	containers := objs[0].Containers()
	if len(containers) != 1 {
		t.Fatalf("got %d containers, want 1", len(containers))
	}
	c := containers[0]
	if got := c.Requests["nvidia.com/gpu"]; got != "1" {
		t.Errorf(`Requests["nvidia.com/gpu"] = %q, want "1" (numeric values normalize to strings)`, got)
	}
	if got := c.Requests["memory"]; got != "16Gi" {
		t.Errorf(`Requests["memory"] = %q, want "16Gi"`, got)
	}
	if got := c.Limits["nvidia.com/gpu"]; got != "2" {
		t.Errorf(`Limits["nvidia.com/gpu"] = %q, want "2"`, got)
	}
}

func TestLookup(t *testing.T) {
	objs := parse(t, `
apiVersion: batch/v1
kind: Job
metadata: {name: j}
spec:
  activeDeadlineSeconds: 3600
  template:
    spec:
      containers: [{name: c}]
`)
	obj := objs[0]
	if v, ok := obj.Lookup("spec", "activeDeadlineSeconds"); !ok || v != 3600 {
		t.Errorf(`Lookup("spec", "activeDeadlineSeconds") = %v, %v; want 3600, true`, v, ok)
	}
	if _, ok := obj.Lookup("spec", "missing"); ok {
		t.Error(`Lookup("spec", "missing") reported ok, want false`)
	}
	if _, ok := obj.Lookup("spec", "activeDeadlineSeconds", "deeper"); ok {
		t.Error(`Lookup through a non-map value reported ok, want false`)
	}
}
