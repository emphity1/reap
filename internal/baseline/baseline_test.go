package baseline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emphity1/reap/internal/rules"
)

func testFindings() []rules.Finding {
	return []rules.Finding{
		{RuleID: "no-gpu-limit", Severity: rules.Error, ObjectRef: "Deployment/ml/a", Detail: "c/nvidia.com/gpu"},
		{RuleID: "job-no-deadline", Severity: rules.Warning, ObjectRef: "Job/ml/b"},
		// Same identity as the first (e.g. two overlays): must dedupe on write.
		{RuleID: "no-gpu-limit", Severity: rules.Error, ObjectRef: "Deployment/ml/a", Detail: "c/nvidia.com/gpu", Source: "other.yaml"},
	}
}

func TestWriteLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".reap-baseline.json")
	n, err := Write(path, testFindings())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 2 {
		t.Errorf("Write wrote %d entries, want 2 (duplicates share a fingerprint)", n)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	for _, want := range []string{`"version": "1"`, `"fingerprint"`, `"ruleId": "no-gpu-limit"`, `"object": "Deployment/ml/a"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("baseline file missing %s:\n%s", want, data)
		}
	}
	ignore, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ignore) != 2 {
		t.Errorf("Load returned %d fingerprints, want 2", len(ignore))
	}
	for _, f := range testFindings() {
		if !ignore[f.Fingerprint()] {
			t.Errorf("fingerprint %s missing from loaded baseline", f.Fingerprint())
		}
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("Load of a missing file must error")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, []byte("not json"), 0o644)
	if _, err := Load(bad); err == nil {
		t.Error("Load of invalid JSON must error")
	}
	wrongVersion := filepath.Join(t.TempDir(), "v2.json")
	os.WriteFile(wrongVersion, []byte(`{"version":"2","findings":[]}`), 0o644)
	if _, err := Load(wrongVersion); err == nil {
		t.Error("Load of an unknown baseline version must error")
	}
}

func TestFilter(t *testing.T) {
	findings := testFindings()
	ignore := map[string]bool{
		findings[0].Fingerprint(): true, // suppresses findings[0] and [2]
		"deadbeefdeadbeef":        true, // stale: matches nothing
	}
	kept, suppressed, stale := Filter(findings, ignore)
	if len(kept) != 1 || kept[0].RuleID != "job-no-deadline" {
		t.Errorf("kept = %v, want only job-no-deadline", kept)
	}
	if suppressed != 2 {
		t.Errorf("suppressed = %d, want 2 (both copies of the shared identity)", suppressed)
	}
	if stale != 1 {
		t.Errorf("stale = %d, want 1", stale)
	}
}
