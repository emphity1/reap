package rules

import (
	"strings"
	"testing"
)

// checkJobDeadline runs only the JobNoDeadline rule over the YAML document.
func checkJobDeadline(t *testing.T, doc string) []Finding {
	t.Helper()
	objs := parseAll(t, doc)
	var findings []Finding
	for _, obj := range objs {
		findings = append(findings, JobNoDeadline{}.Check(obj)...)
	}
	return findings
}

func TestJobNoDeadline(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		fixtureFile  string
		wantFindings int
		wantInMsg    string
	}{
		{
			name:         "bad fixture: GPU job and cronjob without deadline",
			fixtureFile:  "job-no-deadline/bad.yaml",
			wantFindings: 2,
		},
		{
			name:         "good fixture is clean",
			fixtureFile:  "job-no-deadline/good.yaml",
			wantFindings: 0,
		},
		{
			name: "gpu job without deadline is flagged with gpu amount",
			doc: `
apiVersion: batch/v1
kind: Job
metadata: {name: j, namespace: ml}
spec:
  template:
    spec:
      containers:
        - name: c
          resources:
            limits: {nvidia.com/gpu: 8}
`,
			wantFindings: 1,
			wantInMsg:    "nvidia.com/gpu: 8",
		},
		{
			name: "volcano job is not a batch/v1 Job despite the kind name",
			doc: `
apiVersion: batch.volcano.sh/v1alpha1
kind: Job
metadata: {name: vj, namespace: ml}
spec:
  schedulerName: volcano
  minAvailable: 2
  tasks:
    - replicas: 2
      name: worker
      template:
        spec:
          containers:
            - name: worker
              resources:
                limits: {nvidia.com/gpu: 1}
                requests: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "cpu-only job without deadline is out of scope",
			doc: `
apiVersion: batch/v1
kind: Job
metadata: {name: j}
spec:
  template:
    spec:
      containers:
        - name: c
          resources:
            limits: {cpu: "4"}
`,
			wantFindings: 0,
		},
		{
			name: "job-level deadline passes",
			doc: `
apiVersion: batch/v1
kind: Job
metadata: {name: j}
spec:
  activeDeadlineSeconds: 3600
  template:
    spec:
      containers:
        - name: c
          resources:
            limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "pod-level deadline also passes",
			doc: `
apiVersion: batch/v1
kind: Job
metadata: {name: j}
spec:
  template:
    spec:
      activeDeadlineSeconds: 3600
      containers:
        - name: c
          resources:
            limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 0,
		},
		{
			name: "gpu cronjob without deadline is flagged",
			doc: `
apiVersion: batch/v1
kind: CronJob
metadata: {name: cj}
spec:
  schedule: "0 2 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: c
              resources:
                limits: {nvidia.com/gpu: 1}
`,
			wantFindings: 1,
			wantInMsg:    "spawns",
		},
		{
			name: "gpu deployment is not a job",
			doc: `
apiVersion: apps/v1
kind: Deployment
metadata: {name: d}
spec:
  template:
    spec:
      containers:
        - name: c
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
			findings := checkJobDeadline(t, doc)
			if len(findings) != tt.wantFindings {
				t.Fatalf("got %d findings %v, want %d", len(findings), findings, tt.wantFindings)
			}
			for _, f := range findings {
				if f.RuleID != "job-no-deadline" {
					t.Errorf("RuleID = %q, want %q", f.RuleID, "job-no-deadline")
				}
				if f.Severity != Warning {
					t.Errorf("Severity = %v, want Warning", f.Severity)
				}
				if !strings.Contains(f.Message, "activeDeadlineSeconds") {
					t.Errorf("Message = %q, want it to mention activeDeadlineSeconds", f.Message)
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

func TestAllIncludesJobNoDeadline(t *testing.T) {
	for _, r := range All() {
		if r.ID() == "job-no-deadline" {
			return
		}
	}
	t.Error(`All() does not include the "job-no-deadline" rule`)
}
