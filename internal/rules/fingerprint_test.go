package rules

import "testing"

func TestFingerprint(t *testing.T) {
	f := Finding{
		RuleID:    "no-gpu-limit",
		ObjectRef: "Deployment/ml/llm-inference",
		Detail:    "server/nvidia.com/gpu",
	}
	// Golden value: precomputed sha256("reap/v1\0" + ruleID + "\0" + objectRef
	// + "\0" + detail + "\0") truncated to 16 hex chars. Changing it silently
	// invalidates every baseline users have written — hence the frozen value.
	const want = "c3c88f7fa45669b8"
	if got := f.Fingerprint(); got != want {
		t.Fatalf("Fingerprint() = %q, want %q", got, want)
	}

	// Metadata must not influence identity.
	g := f
	g.Severity = Error
	g.Message = "different wording after a copy edit"
	g.Source = "some/other/path.yaml"
	g.Fix = "different suggestion"
	if got := g.Fingerprint(); got != want {
		t.Errorf("metadata changed the fingerprint: %q != %q", got, want)
	}

	// Each identity component must influence it.
	alts := map[string]Finding{
		"rule":   {RuleID: "other-rule", ObjectRef: f.ObjectRef, Detail: f.Detail},
		"object": {RuleID: f.RuleID, ObjectRef: "Deployment/ml/other", Detail: f.Detail},
		"detail": {RuleID: f.RuleID, ObjectRef: f.ObjectRef, Detail: "sidecar/nvidia.com/gpu"},
	}
	for name, alt := range alts {
		if alt.Fingerprint() == want {
			t.Errorf("changing %s did not change the fingerprint", name)
		}
	}

	// An empty namespace is its own identity, never conflated with "default".
	noNS := Finding{RuleID: "r", ObjectRef: "Deployment/web"}
	defaultNS := Finding{RuleID: "r", ObjectRef: "Deployment/default/web"}
	if noNS.Fingerprint() == defaultNS.Fingerprint() {
		t.Error("empty namespace and default namespace share a fingerprint")
	}
}

func TestNoGPULimitSetsSemanticDetail(t *testing.T) {
	findings := checkYAML(t, readFixture(t, "no-gpu-limit/bad.yaml"))
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2", len(findings))
	}
	want := map[string]bool{
		"server/nvidia.com/gpu":  true,
		"trainer/nvidia.com/gpu": true,
	}
	for _, f := range findings {
		if !want[f.Detail] {
			t.Errorf("Detail = %q, want container-name/resource-name (no positional indices)", f.Detail)
		}
	}
}
