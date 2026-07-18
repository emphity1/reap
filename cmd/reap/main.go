// Command reap lints Kubernetes manifests for GPU waste and ML workload
// misconfigurations.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/emphity1/reap/internal/baseline"
	"github.com/emphity1/reap/internal/engine"
	"github.com/emphity1/reap/internal/parser"
	"github.com/emphity1/reap/internal/report"
	"github.com/emphity1/reap/internal/rules"
)

const usage = `usage: reap [flags] <path>...

Lints Kubernetes manifests for GPU waste and ML workload misconfigurations.

Paths may be YAML files, directories (scanned recursively for *.yaml/*.yml),
or "-" to read from stdin:

  reap ./manifests/
  helm template . | reap -

Exit codes: 0 no findings at/above the threshold, 1 findings at/above the
threshold, 2 usage or input error.

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// isTerminal reports whether the writer is an interactive terminal, so
// -color=auto stays plain when output is piped or captured.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reap", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	failOn := fs.String("fail-on", "warning",
		"exit non-zero on findings at or above this severity: info, warning, error, or none")
	format := fs.String("format", "text", "output format: text or json")
	color := fs.String("color", "auto", "colorize text output: auto, always, or never")
	baselinePath := fs.String("baseline", "",
		"baseline file of accepted findings to suppress (see -write-baseline)")
	writeBaseline := fs.String("write-baseline", "",
		"write the current findings to this baseline file and exit 0")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}
	var colorOn bool
	switch *color {
	case "always":
		colorOn = true
	case "never":
		colorOn = false
	case "auto":
		colorOn = isTerminal(stdout) && os.Getenv("NO_COLOR") == ""
	default:
		fmt.Fprintf(stderr, "reap: -color: unknown value %q (valid: auto, always, never)\n", *color)
		return 2
	}
	var reporter report.Reporter
	switch *format {
	case "text":
		reporter = report.Text{Color: colorOn}
	case "json":
		reporter = report.JSON{}
	default:
		fmt.Fprintf(stderr, "reap: -format: unknown format %q (valid: text, json)\n", *format)
		return 2
	}
	var threshold rules.Severity
	if *failOn != "none" {
		t, err := rules.ParseSeverity(*failOn)
		if err != nil {
			fmt.Fprintf(stderr, "reap: -fail-on: %v\n", err)
			return 2
		}
		threshold = t
	}

	objs, err := parser.Load(fs.Args(), stdin)
	if err != nil {
		fmt.Fprintf(stderr, "reap: %v\n", err)
		return 2
	}
	findings := engine.Run(objs, rules.All(), rules.AllSet())

	if *writeBaseline != "" {
		if *baselinePath != "" {
			fmt.Fprintln(stderr, "reap: -baseline and -write-baseline are mutually exclusive")
			return 2
		}
		n, err := baseline.Write(*writeBaseline, findings)
		if err != nil {
			fmt.Fprintf(stderr, "reap: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "reap: wrote %d baseline entr(y/ies) to %s\n", n, *writeBaseline)
		return 0
	}
	suppressed, stale := 0, 0
	if *baselinePath != "" {
		ignore, err := baseline.Load(*baselinePath)
		if err != nil {
			fmt.Fprintf(stderr, "reap: %v\n", err)
			return 2
		}
		findings, suppressed, stale = baseline.Filter(findings, ignore)
	}

	res := report.Result{
		Findings:       findings,
		ObjectsChecked: len(objs),
		Suppressed:     suppressed,
		StaleBaseline:  stale,
	}
	if err := reporter.Report(stdout, res); err != nil {
		fmt.Fprintf(stderr, "reap: %v\n", err)
		return 2
	}

	if *failOn == "none" {
		return 0
	}
	for _, f := range findings {
		if f.Severity >= threshold {
			return 1
		}
	}
	return 0
}
