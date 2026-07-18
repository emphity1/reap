// Package baseline suppresses known findings so reap can be adopted on
// existing repos without failing CI on pre-existing problems. A baseline is
// a set of finding fingerprints: stable identities that survive file moves,
// re-renders, and message rewording.
package baseline

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/emphity1/reap/internal/rules"
)

// fileVersion is the baseline schema version this build reads and writes.
const fileVersion = "1"

// Entry is one accepted finding. Only the fingerprint matters for matching;
// ruleId and object are context for the humans reviewing the file, and
// reason is theirs to fill in.
type Entry struct {
	Fingerprint string `json:"fingerprint"`
	RuleID      string `json:"ruleId,omitempty"`
	Object      string `json:"object,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type file struct {
	Version  string  `json:"version"`
	Findings []Entry `json:"findings"`
}

// Write stores the findings' fingerprints at path, deduplicated and sorted
// for stable diffs, and returns how many entries were written.
func Write(path string, findings []rules.Finding) (int, error) {
	byFingerprint := map[string]Entry{}
	for _, f := range findings {
		fp := f.Fingerprint()
		if _, ok := byFingerprint[fp]; !ok {
			byFingerprint[fp] = Entry{Fingerprint: fp, RuleID: f.RuleID, Object: f.ObjectRef}
		}
	}
	entries := make([]Entry, 0, len(byFingerprint))
	for _, e := range byFingerprint {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Fingerprint < entries[j].Fingerprint })

	data, err := json.MarshalIndent(file{Version: fileVersion, Findings: entries}, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return 0, err
	}
	return len(entries), nil
}

// Load reads a baseline file and returns the set of ignored fingerprints.
func Load(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Version != fileVersion {
		return nil, fmt.Errorf("%s: unsupported baseline version %q (this build supports %q)", path, f.Version, fileVersion)
	}
	ignore := make(map[string]bool, len(f.Findings))
	for _, e := range f.Findings {
		ignore[e.Fingerprint] = true
	}
	return ignore, nil
}

// Filter splits findings into kept and suppressed, and reports how many
// baseline entries matched nothing (stale — candidates for pruning).
func Filter(findings []rules.Finding, ignore map[string]bool) (kept []rules.Finding, suppressed, stale int) {
	matched := map[string]bool{}
	for _, f := range findings {
		fp := f.Fingerprint()
		if ignore[fp] {
			matched[fp] = true
			suppressed++
			continue
		}
		kept = append(kept, f)
	}
	return kept, suppressed, len(ignore) - len(matched)
}
