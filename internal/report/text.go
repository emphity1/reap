package report

import (
	"fmt"
	"io"

	"github.com/emphity1/reap/internal/rules"
)

// Text renders findings as human-readable terminal output, grouped by file
// and object.
type Text struct{}

func (Text) Report(w io.Writer, res Result) error {
	if len(res.Findings) == 0 {
		_, err := fmt.Fprintf(w, "reap: %d object(s) checked, no findings\n", res.ObjectsChecked)
		return err
	}
	counts := map[rules.Severity]int{}
	lastGroup := ""
	for _, f := range res.Findings {
		counts[f.Severity]++
		group := f.Source + ": " + f.ObjectRef
		if group != lastGroup {
			if lastGroup != "" {
				fmt.Fprintln(w)
			}
			fmt.Fprintln(w, group)
			lastGroup = group
		}
		fmt.Fprintf(w, "  [%s] %s: %s\n", f.Severity, f.RuleID, f.Message)
		if f.Fix != "" {
			fmt.Fprintf(w, "        fix: %s\n", f.Fix)
		}
	}
	_, err := fmt.Fprintf(w, "\nreap: %d object(s) checked, %d finding(s): %d error, %d warning, %d info\n",
		res.ObjectsChecked, len(res.Findings), counts[rules.Error], counts[rules.Warning], counts[rules.Info])
	return err
}
