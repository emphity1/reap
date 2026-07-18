package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	badFixture  = "../../testdata/no-gpu-limit/bad.yaml"
	goodFixture = "../../testdata/no-gpu-limit/good.yaml"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		stdin      string
		wantExit   int
		wantStdout []string // substrings that must appear on stdout
		wantStderr string   // substring that must appear on stderr, if any
	}{
		{
			name:       "failing manifest exits 1 and reports the finding",
			args:       []string{badFixture},
			wantExit:   1,
			wantStdout: []string{"no-gpu-limit", "[error]", "Deployment/ml/llm-inference", "fix:"},
		},
		{
			name:       "clean manifest exits 0",
			args:       []string{goodFixture},
			wantExit:   0,
			wantStdout: []string{"no findings"},
		},
		{
			name:       "reads manifests from stdin via dash",
			args:       []string{"-"},
			stdin:      "fixture:bad", // replaced with file content in the test body
			wantExit:   1,
			wantStdout: []string{"no-gpu-limit", "stdin"},
		},
		{
			name:       "directory argument is scanned recursively",
			args:       []string{"../../testdata"},
			wantExit:   1,
			wantStdout: []string{"no-gpu-limit", "job-no-deadline", "[warning]"},
		},
		{
			name:     "fail-on none reports findings but exits 0",
			args:     []string{"-fail-on=none", badFixture},
			wantExit: 0,
			wantStdout: []string{
				"no-gpu-limit",
			},
		},
		{
			name:       "missing file exits 2",
			args:       []string{"does-not-exist.yaml"},
			wantExit:   2,
			wantStderr: "does-not-exist.yaml",
		},
		{
			name:       "no arguments prints usage and exits 2",
			args:       nil,
			wantExit:   2,
			wantStderr: "usage",
		},
		{
			name:       "invalid fail-on value exits 2",
			args:       []string{"-fail-on=bogus", goodFixture},
			wantExit:   2,
			wantStderr: "bogus",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := tt.stdin
			if stdin == "fixture:bad" {
				data, err := os.ReadFile(filepath.FromSlash(badFixture))
				if err != nil {
					t.Fatalf("read fixture: %v", err)
				}
				stdin = string(data)
			}
			var stdout, stderr strings.Builder
			exit := run(tt.args, strings.NewReader(stdin), &stdout, &stderr)
			if exit != tt.wantExit {
				t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s",
					exit, tt.wantExit, stdout.String(), stderr.String())
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("stdout missing %q:\n%s", want, stdout.String())
				}
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr missing %q:\n%s", tt.wantStderr, stderr.String())
			}
		})
	}
}

func TestRunJSONFormat(t *testing.T) {
	var stdout, stderr strings.Builder
	exit := run([]string{"-format=json", badFixture}, strings.NewReader(""), &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("exit = %d, want 1\nstderr:\n%s", exit, stderr.String())
	}
	var out struct {
		Version string `json:"version"`
		Summary struct {
			Findings int `json:"findings"`
		} `json:"summary"`
		Findings []struct {
			RuleID      string `json:"ruleId"`
			Fingerprint string `json:"fingerprint"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &out); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if out.Summary.Findings != 3 || len(out.Findings) != 3 {
		t.Fatalf("got %d findings (summary %d), want 3", len(out.Findings), out.Summary.Findings)
	}
	for _, f := range out.Findings {
		if len(f.Fingerprint) != 16 {
			t.Errorf("fingerprint %q has length %d, want 16", f.Fingerprint, len(f.Fingerprint))
		}
	}
}

func TestRunInvalidFormat(t *testing.T) {
	var stdout, stderr strings.Builder
	exit := run([]string{"-format=xml", goodFixture}, strings.NewReader(""), &stdout, &stderr)
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	if !strings.Contains(stderr.String(), "xml") {
		t.Errorf("stderr should name the invalid format:\n%s", stderr.String())
	}
}
