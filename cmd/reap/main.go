// Command reap lints Kubernetes manifests for GPU waste and ML workload
// misconfigurations.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

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

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reap", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	failOn := fs.String("fail-on", "warning",
		"exit non-zero on findings at or above this severity: info, warning, error, or none")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
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
	findings := engine.Run(objs, rules.All())
	res := report.Result{Findings: findings, ObjectsChecked: len(objs)}
	if err := (report.Text{}).Report(stdout, res); err != nil {
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
