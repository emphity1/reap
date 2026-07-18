package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/emphity1/reap/internal/rules"
)

func render(t *testing.T, r Text, res Result) string {
	t.Helper()
	var buf bytes.Buffer
	if err := r.Report(&buf, res); err != nil {
		t.Fatalf("Report: %v", err)
	}
	return buf.String()
}

func sampleFindings() []rules.Finding {
	return []rules.Finding{
		{
			RuleID: "no-gpu-limit", Severity: rules.Error,
			Message:   "container \"server\" requests nvidia.com/gpu: 1 but sets no limit; Kubernetes rejects GPU requests without an equal limit, so this manifest will not deploy",
			ObjectRef: "Deployment/ml/llm-inference", Source: "manifests/app.yaml",
			Fix: "set resources.limits[\"nvidia.com/gpu\"] equal to the request",
		},
		{
			RuleID: "job-no-deadline", Severity: rules.Warning,
			Message:   "Job \"finetune\" holds nvidia.com/gpu: 2 with no activeDeadlineSeconds; if a run hangs, those GPUs stay allocated — nights and weekends included — while doing no work",
			ObjectRef: "Job/ml/finetune", Source: "manifests/app.yaml",
			Fix: "set spec.activeDeadlineSeconds to a hard upper bound for the run",
		},
	}
}

func TestTextGroupsByFileThenObject(t *testing.T) {
	out := render(t, Text{}, Result{Findings: sampleFindings(), ObjectsChecked: 2})
	if got := strings.Count(out, "manifests/app.yaml"); got != 1 {
		t.Errorf("source path printed %d times, want once as a group header:\n%s", got, out)
	}
	for _, want := range []string{"Deployment/ml/llm-inference", "Job/ml/finetune", "[error] no-gpu-limit", "[warning] job-no-deadline", "fix:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// The object header is indented under the file, the finding under the object.
	if !strings.Contains(out, "\n  Deployment/ml/llm-inference\n") {
		t.Errorf("object header not indented under the file header:\n%s", out)
	}
	if !strings.Contains(out, "\n    [error] no-gpu-limit\n") {
		t.Errorf("severity tag not on its own indented line:\n%s", out)
	}
}

func TestTextWrapsLongMessages(t *testing.T) {
	out := render(t, Text{Width: 100}, Result{Findings: sampleFindings(), ObjectsChecked: 2})
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 100 {
			t.Errorf("line longer than 100 columns (%d):\n%s", len(line), line)
		}
	}
}

func TestTextColor(t *testing.T) {
	colored := render(t, Text{Color: true}, Result{Findings: sampleFindings(), ObjectsChecked: 2})
	if !strings.Contains(colored, "\x1b[31m") || !strings.Contains(colored, "\x1b[0m") {
		t.Errorf("colored output missing ANSI codes:\n%q", colored)
	}
	plain := render(t, Text{}, Result{Findings: sampleFindings(), ObjectsChecked: 2})
	if strings.Contains(plain, "\x1b[") {
		t.Errorf("plain output contains ANSI codes:\n%q", plain)
	}
}

func TestTextSummaryPhrasing(t *testing.T) {
	out := render(t, Text{}, Result{Findings: sampleFindings(), ObjectsChecked: 2})
	if !strings.Contains(out, "2 objects checked, 2 findings (1 error, 1 warning, 0 info)") {
		t.Errorf("summary phrasing wrong:\n%s", out)
	}
	one := render(t, Text{}, Result{Findings: sampleFindings()[:1], ObjectsChecked: 1})
	if !strings.Contains(one, "1 object checked, 1 finding (1 error, 0 warning, 0 info)") {
		t.Errorf("singular phrasing wrong:\n%s", one)
	}
	clean := render(t, Text{}, Result{ObjectsChecked: 3, Suppressed: 4, StaleBaseline: 2})
	if !strings.Contains(clean, "3 objects checked, no findings (4 suppressed by baseline, 2 stale entries)") {
		t.Errorf("baseline note phrasing wrong:\n%s", clean)
	}
}
