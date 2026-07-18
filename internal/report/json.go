package report

import (
	"encoding/json"
	"io"

	"github.com/emphity1/reap/internal/rules"
)

// JSON renders findings as a machine-readable document for CI automation.
// Schema version "1"; the fingerprint field is the finding's stable identity,
// which the baseline mechanism consumes.
type JSON struct{}

type jsonOutput struct {
	Version  string        `json:"version"`
	Summary  jsonSummary   `json:"summary"`
	Findings []jsonFinding `json:"findings"`
}

type jsonSummary struct {
	ObjectsChecked int            `json:"objectsChecked"`
	Findings       int            `json:"findings"`
	BySeverity     map[string]int `json:"bySeverity"`
	Suppressed     int            `json:"suppressed"`
	StaleBaseline  int            `json:"staleBaselineEntries"`
}

type jsonFinding struct {
	RuleID      string `json:"ruleId"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Object      string `json:"object"`
	Detail      string `json:"detail,omitempty"`
	Source      string `json:"source"`
	Fix         string `json:"fix,omitempty"`
	Fingerprint string `json:"fingerprint"`
}

func (JSON) Report(w io.Writer, res Result) error {
	out := jsonOutput{
		Version: "1",
		Summary: jsonSummary{
			ObjectsChecked: res.ObjectsChecked,
			Findings:       len(res.Findings),
			Suppressed:     res.Suppressed,
			StaleBaseline:  res.StaleBaseline,
			BySeverity: map[string]int{
				rules.Error.String():   0,
				rules.Warning.String(): 0,
				rules.Info.String():    0,
			},
		},
		Findings: make([]jsonFinding, 0, len(res.Findings)),
	}
	for _, f := range res.Findings {
		out.Summary.BySeverity[f.Severity.String()]++
		out.Findings = append(out.Findings, jsonFinding{
			RuleID:      f.RuleID,
			Severity:    f.Severity.String(),
			Message:     f.Message,
			Object:      f.ObjectRef,
			Detail:      f.Detail,
			Source:      f.Source,
			Fix:         f.Fix,
			Fingerprint: f.Fingerprint(),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
