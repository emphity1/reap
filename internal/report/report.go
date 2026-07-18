// Package report renders findings for humans and machines. Additional
// formats (json, sarif) implement the same Reporter interface.
package report

import (
	"io"

	"github.com/emphity1/reap/internal/rules"
)

// Result is everything a reporter needs to render one run.
type Result struct {
	Findings       []rules.Finding
	ObjectsChecked int
	Suppressed     int // findings hidden by the baseline
	StaleBaseline  int // baseline entries that matched nothing
}

// Reporter renders a Result to a writer.
type Reporter interface {
	Report(w io.Writer, res Result) error
}
