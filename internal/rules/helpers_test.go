package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emphity1/reap/internal/parser"
)

// parseAll decodes a YAML document into objects, failing the test on error.
func parseAll(t *testing.T, doc string) []parser.Object {
	t.Helper()
	objs, err := parser.Parse(strings.NewReader(doc), "test.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return objs
}

// readFixture loads a manifest from testdata by its path relative to the
// testdata root, e.g. "no-gpu-limit/bad.yaml".
func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(data)
}
