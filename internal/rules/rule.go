// Package rules defines the lint rules and the types they share.
package rules

import (
	"fmt"

	"github.com/emphity1/reap/internal/parser"
)

// Severity ranks how bad a finding is. The zero value is Info; higher values
// are worse, so severities compare with < and >.
type Severity int

const (
	Info Severity = iota
	Warning
	Error
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warning:
		return "warning"
	case Error:
		return "error"
	}
	return fmt.Sprintf("severity(%d)", int(s))
}

// ParseSeverity converts a flag value such as "warning" into a Severity.
func ParseSeverity(s string) (Severity, error) {
	switch s {
	case "info":
		return Info, nil
	case "warning":
		return Warning, nil
	case "error":
		return Error, nil
	}
	return 0, fmt.Errorf("unknown severity %q (valid: info, warning, error)", s)
}

// Finding is one problem detected in one object.
type Finding struct {
	RuleID    string
	Severity  Severity
	Message   string // human-readable, actionable, cost-aware when possible
	ObjectRef string // Kind/Namespace/Name of the offending object
	Source    string // file the object came from ("stdin" for piped input)
	Fix       string // suggested remediation, empty if none
}

// Rule checks one object at a time. Rules must be stateless: the engine may
// call Check in any order over any number of objects.
type Rule interface {
	ID() string
	Severity() Severity
	Check(obj parser.Object) []Finding
}

// All returns every registered rule.
func All() []Rule {
	return []Rule{
		NoGPULimit{},
		JobNoDeadline{},
	}
}
