package parser

import (
	"fmt"
	"sort"
)

// Object is a single Kubernetes manifest document in normalized form. Raw
// holds the full decoded document so rules can inspect any field, including
// fields of CRDs the parser does not model explicitly.
type Object struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
	Source     string // file path the object came from, or "stdin"
	Raw        map[string]any
}

// Ref identifies the object as Kind/Namespace/Name; the namespace is omitted
// when the manifest does not set one.
func (o Object) Ref() string {
	if o.Namespace == "" {
		return o.Kind + "/" + o.Name
	}
	return o.Kind + "/" + o.Namespace + "/" + o.Name
}

// Lookup walks the raw document along the given map keys and returns the
// value at the end of the path, so rules can read fields the parser does not
// model, e.g. obj.Lookup("spec", "activeDeadlineSeconds").
func (o Object) Lookup(path ...string) (any, bool) {
	var cur any = o.Raw
	for _, key := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[key]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// Container is a normalized view over a container or initContainer.
// Resource quantities are rendered as strings regardless of how YAML typed
// them (nvidia.com/gpu: 1 and nvidia.com/gpu: "1" compare equal).
type Container struct {
	Name     string
	Init     bool
	Requests map[string]string
	Limits   map[string]string
}

// podSpecPaths locates the pod spec inside the core workload kinds.
var podSpecPaths = map[string][]string{
	"Pod":         {"spec"},
	"Deployment":  {"spec", "template", "spec"},
	"ReplicaSet":  {"spec", "template", "spec"},
	"StatefulSet": {"spec", "template", "spec"},
	"DaemonSet":   {"spec", "template", "spec"},
	"Job":         {"spec", "template", "spec"},
	"CronJob":     {"spec", "jobTemplate", "spec", "template", "spec"},
}

// Containers returns every container and initContainer the object defines.
// Core workload kinds are read at their known pod-spec paths; any other kind
// (CRDs such as PyTorchJob or MPIJob) is searched for nested
// template.spec.containers blocks, which covers the replica-spec layout those
// CRDs embed pod templates in.
func (o Object) Containers() []Container {
	var specs []map[string]any
	if path, ok := podSpecPaths[o.Kind]; ok {
		if spec, ok := dig(o.Raw, path...); ok {
			specs = append(specs, spec)
		}
	} else if spec, ok := o.Raw["spec"].(map[string]any); ok {
		specs = findPodSpecs(spec)
	}
	var out []Container
	for _, spec := range specs {
		out = append(out, containersFrom(spec, "containers", false)...)
		out = append(out, containersFrom(spec, "initContainers", true)...)
	}
	return out
}

func dig(m map[string]any, path ...string) (map[string]any, bool) {
	cur := m
	for _, key := range path {
		next, ok := cur[key].(map[string]any)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// findPodSpecs walks nested maps collecting every template.spec that holds a
// containers list. Keys are visited in sorted order so findings are
// deterministic across runs.
func findPodSpecs(m map[string]any) []map[string]any {
	var out []map[string]any
	if tmpl, ok := m["template"].(map[string]any); ok {
		if spec, ok := tmpl["spec"].(map[string]any); ok {
			if _, ok := spec["containers"].([]any); ok {
				out = append(out, spec)
			}
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if sub, ok := m[k].(map[string]any); ok {
			out = append(out, findPodSpecs(sub)...)
		}
	}
	return out
}

func containersFrom(spec map[string]any, key string, init bool) []Container {
	list, ok := spec[key].([]any)
	if !ok {
		return nil
	}
	var out []Container
	for _, item := range list {
		cm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c := Container{Init: init}
		c.Name, _ = cm["name"].(string)
		if res, ok := cm["resources"].(map[string]any); ok {
			c.Requests = quantities(res["requests"])
			c.Limits = quantities(res["limits"])
		}
		out = append(out, c)
	}
	return out
}

func quantities(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		out[k] = fmt.Sprint(val)
	}
	return out
}
