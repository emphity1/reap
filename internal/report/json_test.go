package report

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/emphity1/reap/internal/rules"
)

// decoded mirrors the documented JSON schema for assertions.
type decoded struct {
	Version string `json:"version"`
	Summary struct {
		ObjectsChecked int            `json:"objectsChecked"`
		Findings       int            `json:"findings"`
		BySeverity     map[string]int `json:"bySeverity"`
	} `json:"summary"`
	Findings []struct {
		RuleID      string `json:"ruleId"`
		Severity    string `json:"severity"`
		Message     string `json:"message"`
		Object      string `json:"object"`
		Detail      string `json:"detail"`
		Source      string `json:"source"`
		Fix         string `json:"fix"`
		Fingerprint string `json:"fingerprint"`
	} `json:"findings"`
}

func TestJSONReport(t *testing.T) {
	finding := rules.Finding{
		RuleID:    "no-gpu-limit",
		Severity:  rules.Error,
		Message:   "container requests GPU without limit",
		ObjectRef: "Deployment/ml/llm-inference",
		Detail:    "server/nvidia.com/gpu",
		Source:    "manifests/app.yaml",
		Fix:       "set the limit",
	}
	var buf bytes.Buffer
	err := JSON{}.Report(&buf, Result{Findings: []rules.Finding{finding}, ObjectsChecked: 3})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	var out decoded
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if out.Version != "1" {
		t.Errorf("version = %q, want %q", out.Version, "1")
	}
	if out.Summary.ObjectsChecked != 3 || out.Summary.Findings != 1 {
		t.Errorf("summary = %+v, want objectsChecked 3, findings 1", out.Summary)
	}
	if out.Summary.BySeverity["error"] != 1 {
		t.Errorf(`bySeverity["error"] = %d, want 1`, out.Summary.BySeverity["error"])
	}
	if len(out.Findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(out.Findings))
	}
	got := out.Findings[0]
	if got.RuleID != finding.RuleID || got.Message != finding.Message ||
		got.Object != finding.ObjectRef || got.Detail != finding.Detail ||
		got.Source != finding.Source || got.Fix != finding.Fix {
		t.Errorf("finding fields = %+v, want them to mirror %+v", got, finding)
	}
	if got.Severity != "error" {
		t.Errorf("severity = %q, want %q (a string, not a number)", got.Severity, "error")
	}
	if got.Fingerprint != finding.Fingerprint() {
		t.Errorf("fingerprint = %q, want %q", got.Fingerprint, finding.Fingerprint())
	}
}

func TestJSONReportEmptyFindingsIsArray(t *testing.T) {
	var buf bytes.Buffer
	if err := (JSON{}).Report(&buf, Result{ObjectsChecked: 2}); err != nil {
		t.Fatalf("Report: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"findings": []`)) {
		t.Errorf("empty findings must serialize as [], not null:\n%s", buf.String())
	}
}
