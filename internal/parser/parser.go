// Package parser reads Kubernetes YAML manifests into normalized objects.
package parser

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads manifests from the given paths. A path may be a YAML file, a
// directory (scanned recursively for *.yaml and *.yml), or "-" to read from
// stdin, so `helm template . | reap -` works.
func Load(paths []string, stdin io.Reader) ([]Object, error) {
	var objs []Object
	for _, path := range paths {
		if path == "-" {
			parsed, err := Parse(stdin, "stdin")
			if err != nil {
				return nil, err
			}
			objs = append(objs, parsed...)
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			parsed, err := loadDir(path)
			if err != nil {
				return nil, err
			}
			objs = append(objs, parsed...)
			continue
		}
		parsed, err := loadFile(path)
		if err != nil {
			return nil, err
		}
		objs = append(objs, parsed...)
	}
	return objs, nil
}

func loadDir(dir string) ([]Object, error) {
	var objs []Object
	found := false
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ext := filepath.Ext(path); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		found = true
		parsed, err := loadFile(path)
		if err != nil {
			return err
		}
		objs = append(objs, parsed...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("no YAML manifests found in %s", dir)
	}
	return objs, nil
}

func loadFile(path string) ([]Object, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f, path)
}

// Parse decodes a (possibly multi-document) YAML stream into Objects. Empty
// documents and documents without a kind are skipped, so the output of
// `helm template` parses as-is. v1 List documents are flattened.
func Parse(r io.Reader, source string) ([]Object, error) {
	dec := yaml.NewDecoder(r)
	var objs []Object
	for i := 1; ; i++ {
		var doc any
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return objs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: document %d: %w", source, i, err)
		}
		if m, ok := doc.(map[string]any); ok {
			objs = append(objs, fromMap(m, source)...)
		}
	}
}

func fromMap(m map[string]any, source string) []Object {
	kind, _ := m["kind"].(string)
	if kind == "" {
		return nil
	}
	if kind == "List" {
		items, _ := m["items"].([]any)
		var out []Object
		for _, item := range items {
			if im, ok := item.(map[string]any); ok {
				out = append(out, fromMap(im, source)...)
			}
		}
		return out
	}
	obj := Object{Kind: kind, Source: source, Raw: m}
	obj.APIVersion, _ = m["apiVersion"].(string)
	if md, ok := m["metadata"].(map[string]any); ok {
		obj.Name, _ = md["name"].(string)
		obj.Namespace, _ = md["namespace"].(string)
	}
	return []Object{obj}
}
