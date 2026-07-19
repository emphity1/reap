package parser

import (
	"fmt"
	"sort"
	"strings"
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

// Group returns the API group of the object's apiVersion — "apps" for
// apps/v1, "" for core v1. Kind names alone are ambiguous: Volcano's Job
// (batch.volcano.sh) is not batch/v1 Job, and rules or path lookups keyed on
// kind must disambiguate with the group.
func (o Object) Group() string {
	if i := strings.IndexByte(o.APIVersion, '/'); i >= 0 {
		return o.APIVersion[:i]
	}
	return ""
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
// them (nvidia.com/gpu: 1 and nvidia.com/gpu: "1" compare equal). Raw holds
// the full container map so rules can inspect fields the parser does not
// model (probes, ports, volumeMounts, env).
type Container struct {
	Name     string
	Image    string
	Init     bool
	Requests map[string]string
	Limits   map[string]string
	Raw      map[string]any
}

// podSpecPaths locates the pod spec inside the core workload kinds. Each
// entry names the API group the kind belongs to: a same-named kind from
// another group (Volcano's batch.volcano.sh Job) is a different type with a
// different layout and must take the CRD fallback instead.
var podSpecPaths = map[string]struct {
	group string
	path  []string
}{
	"Pod":         {"", []string{"spec"}},
	"Deployment":  {"apps", []string{"spec", "template", "spec"}},
	"ReplicaSet":  {"apps", []string{"spec", "template", "spec"}},
	"StatefulSet": {"apps", []string{"spec", "template", "spec"}},
	"DaemonSet":   {"apps", []string{"spec", "template", "spec"}},
	"Job":         {"batch", []string{"spec", "template", "spec"}},
	"CronJob":     {"batch", []string{"spec", "jobTemplate", "spec", "template", "spec"}},
}

// PodSpec is one pod spec found inside the object, with pod-level fields
// (volumes, nodeSelector, tolerations, affinity, ...) reachable through Raw
// and its containers already normalized.
type PodSpec struct {
	Raw        map[string]any
	Containers []Container
}

// PodSpecs returns every pod spec the object defines. Core workload kinds
// are read at their known pod-spec paths; any other kind (CRDs such as
// PyTorchJob or MPIJob) is searched for nested template.spec.containers
// blocks, which covers the replica-spec layout those CRDs embed pod
// templates in.
func (o Object) PodSpecs() []PodSpec {
	var specs []map[string]any
	if known, ok := podSpecPaths[o.Kind]; ok && o.Group() == known.group {
		if spec, ok := dig(o.Raw, known.path...); ok {
			specs = append(specs, spec)
		}
	} else if spec, ok := o.Raw["spec"].(map[string]any); ok {
		specs = findPodSpecs(spec)
	}
	out := make([]PodSpec, 0, len(specs))
	for _, spec := range specs {
		ps := PodSpec{Raw: spec}
		ps.Containers = append(ps.Containers, containersFrom(spec, "containers", false)...)
		ps.Containers = append(ps.Containers, containersFrom(spec, "initContainers", true)...)
		out = append(out, ps)
	}
	return out
}

// Containers returns every container and initContainer across the object's
// pod specs.
func (o Object) Containers() []Container {
	var out []Container
	for _, ps := range o.PodSpecs() {
		out = append(out, ps.Containers...)
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

// findPodSpecs walks nested maps and lists collecting every template.spec
// that holds a containers list. Lists matter as much as maps: CRDs commonly
// nest pod templates inside arrays (RayCluster workerGroupSpecs, Volcano Job
// tasks). Keys are visited in sorted order so findings are deterministic
// across runs.
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
		switch sub := m[k].(type) {
		case map[string]any:
			out = append(out, findPodSpecs(sub)...)
		case []any:
			for _, item := range sub {
				if im, ok := item.(map[string]any); ok {
					out = append(out, findPodSpecs(im)...)
				}
			}
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
		c := Container{Init: init, Raw: cm}
		c.Name, _ = cm["name"].(string)
		c.Image, _ = cm["image"].(string)
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
