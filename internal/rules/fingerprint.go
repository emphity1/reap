package rules

import (
	"crypto/sha256"
	"encoding/hex"
)

// fingerprintVersion is baked into every hash: bumping it deliberately
// invalidates all existing baselines, so bump only for breaking identity
// changes.
const fingerprintVersion = "reap/v1"

// Fingerprint returns a stable identity for the finding: a truncated hash of
// rule ID, object reference, and the rule's semantic detail key. Message,
// severity, fix, and file path are metadata and never enter the hash, so the
// identity survives copy edits, re-renders, and file moves. Identical logical
// findings from different files (e.g. two overlays defining the same object
// with the same violation) share a fingerprint by design: baselining one
// baselines both.
func (f Finding) Fingerprint() string {
	h := sha256.New()
	for _, part := range []string{fingerprintVersion, f.RuleID, f.ObjectRef, f.Detail} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
