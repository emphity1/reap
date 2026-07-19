package engine

import (
	"testing"

	"github.com/emphity1/reap/internal/parser"
	"github.com/emphity1/reap/internal/rules"
)

// stubRule reports one finding per object so ordering can be observed.
type stubRule struct {
	id  string
	sev rules.Severity
}

func (r stubRule) ID() string               { return r.id }
func (r stubRule) Severity() rules.Severity { return r.sev }

func (r stubRule) Check(obj parser.Object) []rules.Finding {
	return []rules.Finding{{
		RuleID:    r.id,
		Severity:  r.Severity(),
		ObjectRef: obj.Ref(),
		Source:    obj.Source,
	}}
}

func TestRunSortsFindings(t *testing.T) {
	objs := []parser.Object{
		{Kind: "Pod", Name: "b", Source: "z.yaml"},
		{Kind: "Pod", Name: "a", Source: "a.yaml"},
	}
	findings := Run(objs, []rules.Rule{stubRule{id: "r2"}, stubRule{id: "r1"}}, nil)
	if len(findings) != 4 {
		t.Fatalf("got %d findings, want 4", len(findings))
	}
	wantOrder := []struct{ source, rule string }{
		{"a.yaml", "r1"},
		{"a.yaml", "r2"},
		{"z.yaml", "r1"},
		{"z.yaml", "r2"},
	}
	for i, want := range wantOrder {
		if findings[i].Source != want.source || findings[i].RuleID != want.rule {
			t.Errorf("finding %d = %s/%s, want %s/%s",
				i, findings[i].Source, findings[i].RuleID, want.source, want.rule)
		}
	}
}

func TestRunOrdersBySeverityWithinObject(t *testing.T) {
	objs := []parser.Object{{Kind: "Pod", Name: "p", Source: "a.yaml"}}
	// Rule IDs chosen so alphabetical order would put the info finding
	// first: severity must outrank rule ID inside one object.
	findings := Run(objs, []rules.Rule{
		stubRule{id: "aaa-info", sev: rules.Info},
		stubRule{id: "zzz-warning", sev: rules.Warning},
		stubRule{id: "mmm-error", sev: rules.Error},
	}, nil)
	if len(findings) != 3 {
		t.Fatalf("got %d findings, want 3", len(findings))
	}
	wantOrder := []string{"mmm-error", "zzz-warning", "aaa-info"}
	for i, want := range wantOrder {
		if findings[i].RuleID != want {
			t.Errorf("finding %d = %s, want %s (severity must sort before rule ID)",
				i, findings[i].RuleID, want)
		}
	}
}

// stubSetRule reports one finding over the whole set.
type stubSetRule struct{}

func (stubSetRule) ID() string               { return "stub-set" }
func (stubSetRule) Severity() rules.Severity { return rules.Info }

func (r stubSetRule) CheckAll(objs []parser.Object, idx *rules.Index) []rules.Finding {
	return []rules.Finding{{RuleID: r.ID(), Severity: r.Severity(), Message: "set", Source: "a.yaml"}}
}

func TestRunDispatchesSetRules(t *testing.T) {
	objs := []parser.Object{{Kind: "Pod", Name: "a", Source: "a.yaml"}}
	findings := Run(objs, []rules.Rule{stubRule{id: "r1"}}, []rules.SetRule{stubSetRule{}})
	var ids []string
	for _, f := range findings {
		ids = append(ids, f.RuleID)
	}
	if len(findings) != 2 {
		t.Fatalf("got findings %v, want one per-object and one set-level", ids)
	}
}
