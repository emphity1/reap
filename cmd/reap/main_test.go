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
		name          string
		args          []string
		stdin         string
		wantExit      int
		wantStdout    []string // substrings that must appear on stdout
		wantNotStdout []string // substrings that must NOT appear on stdout
		wantStderr    string   // substring that must appear on stderr, if any
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
			name:     "directory argument is scanned recursively",
			args:     []string{"../../testdata"},
			wantExit: 1,
			wantStdout: []string{"no-gpu-limit", "job-no-deadline", "model-server-no-probes", "distributed-training-no-gang-scheduling", "shm-too-small", "gpu-no-node-targeting", "multinode-no-topology-affinity", "notebook-no-idle-culling", "inference-no-hpa", "no-pdb", "[warning]", "[info]",
				// Pod specs nested in arrays (RayCluster workerGroupSpecs)
				// must be visible: dogfood regression.
				"RayCluster/ray-array/array-gpu-workers"},
			// The KServe control plane must not be detected as a model
			// server: dogfood regression.
			wantNotStdout: []string{"kserve-controller"},
		},
		{
			name:       "info findings do not fail the default warning threshold",
			args:       []string{"../../testdata/gpu-no-node-targeting/bad.yaml"},
			wantExit:   0,
			wantStdout: []string{"gpu-no-node-targeting", "[info]"},
		},
		{
			name:       "fail-on info gates on info findings",
			args:       []string{"-fail-on=info", "../../testdata/gpu-no-node-targeting/bad.yaml"},
			wantExit:   1,
			wantStdout: []string{"gpu-no-node-targeting"},
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
			for _, notWant := range tt.wantNotStdout {
				if strings.Contains(stdout.String(), notWant) {
					t.Errorf("stdout must not contain %q:\n%s", notWant, stdout.String())
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
	if out.Summary.Findings != 2 || len(out.Findings) != 2 {
		t.Fatalf("got %d findings (summary %d), want 2", len(out.Findings), out.Summary.Findings)
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

func TestBaselineFlow(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, ".reap-baseline.json")

	var out, errOut strings.Builder
	if exit := run([]string{"-write-baseline", baselinePath, badFixture}, strings.NewReader(""), &out, &errOut); exit != 0 {
		t.Fatalf("write-baseline exit = %d, want 0\nstderr:\n%s", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "2 baseline entries") {
		t.Errorf("write-baseline output should count entries with proper pluralization:\n%s", out.String())
	}
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("baseline file not written: %v", err)
	}

	out.Reset()
	if exit := run([]string{"-baseline", baselinePath, badFixture}, strings.NewReader(""), &out, &errOut); exit != 0 {
		t.Fatalf("lint with baseline exit = %d, want 0 (all findings suppressed)\nstdout:\n%s", exit, out.String())
	}
	if !strings.Contains(out.String(), "suppressed") {
		t.Errorf("output should report suppressed findings:\n%s", out.String())
	}

	out.Reset()
	otherFixture := "../../testdata/job-no-deadline/bad.yaml"
	if exit := run([]string{"-baseline", baselinePath, otherFixture}, strings.NewReader(""), &out, &errOut); exit != 1 {
		t.Fatalf("baseline for another file must not suppress: exit = %d, want 1\nstdout:\n%s", exit, out.String())
	}
	if !strings.Contains(out.String(), "stale") {
		t.Errorf("output should report stale baseline entries:\n%s", out.String())
	}

	if exit := run([]string{"-baseline", filepath.Join(dir, "missing.json"), badFixture}, strings.NewReader(""), &out, &errOut); exit != 2 {
		t.Errorf("missing baseline file: exit = %d, want 2", exit)
	}
	if exit := run([]string{"-baseline", baselinePath, "-write-baseline", baselinePath, badFixture}, strings.NewReader(""), &out, &errOut); exit != 2 {
		t.Errorf("baseline together with write-baseline: exit = %d, want 2", exit)
	}
}

func TestColorFlag(t *testing.T) {
	var out, errOut strings.Builder
	if exit := run([]string{"-color=always", badFixture}, strings.NewReader(""), &out, &errOut); exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if !strings.Contains(out.String(), "\x1b[31m") {
		t.Errorf("-color=always output has no ANSI codes:\n%q", out.String())
	}
	out.Reset()
	if exit := run([]string{"-color=never", badFixture}, strings.NewReader(""), &out, &errOut); exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Errorf("-color=never output contains ANSI codes:\n%q", out.String())
	}
	// auto on a non-TTY writer (this test) must not color
	out.Reset()
	run([]string{badFixture}, strings.NewReader(""), &out, &errOut)
	if strings.Contains(out.String(), "\x1b[") {
		t.Errorf("auto color on non-TTY output contains ANSI codes:\n%q", out.String())
	}
	if exit := run([]string{"-color=rainbow", badFixture}, strings.NewReader(""), &out, &errOut); exit != 2 {
		t.Errorf("invalid -color value: exit = %d, want 2", exit)
	}
}
