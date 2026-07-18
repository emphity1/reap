package engine

import (
	"testing"

	"github.com/emphity1/reap/internal/parser"
	"github.com/emphity1/reap/internal/rules"
)

// stubRule reports one finding per object so ordering can be observed.
type stubRule struct{ id string }

func (r stubRule) ID() string               { return r.id }
func (r stubRule) Severity() rules.Severity { return rules.Warning }

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
