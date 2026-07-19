package rules

import (
	"fmt"
	"strings"

	"github.com/emphity1/reap/internal/parser"
)

// ShmTooSmall flags GPU containers with no memory-backed /dev/shm mount.
// Kubernetes gives every container a 64 MB /dev/shm by default; PyTorch
// DataLoader workers and NCCL/tensor-parallel inference exchange tensors
// through shared memory and crash into cryptic bus errors ("unable to write
// to shared memory", "DataLoader worker killed") the first time a batch
// doesn't fit, and Triton's Python backend degrades the same way. One of the
// most common and least obvious ML-on-Kubernetes failures.
//
// Breadth decision (settled after the 2026-07 dogfood run): the rule stays
// broad — every GPU container — because the fix is cheap and harmless, but
// the message no longer overclaims PyTorch specifics and the fix names the
// escape hatch: runtimes that never touch shared memory (llama.cpp-based
// servers like ollama were the one debatable firing in the corpus) should
// baseline the finding rather than get a narrower detector that silently
// misses real cases.
type ShmTooSmall struct{}

func (ShmTooSmall) ID() string { return "shm-too-small" }

func (ShmTooSmall) Severity() Severity { return Warning }

func (r ShmTooSmall) Check(obj parser.Object) []Finding {
	// Ray CRDs are excluded: the KubeRay operator injects a memory-backed
	// emptyDir at /dev/shm into every pod it builds from a
	// RayCluster/RayJob/RayService (ray-operator common/pod.go —
	// unconditional at v1.4.2, skipped only for an explicit
	// plasma-directory on newer versions), so the manifest-level absence
	// is present at system level. Same rationale as the gang rule's Ray
	// exclusion: operator-level behavior invisible in the manifest.
	if obj.Group() == "ray.io" {
		return nil
	}
	var findings []Finding
	for _, ps := range obj.PodSpecs() {
		if hostIPC, _ := ps.Raw["hostIPC"].(bool); hostIPC {
			continue // shares the host's /dev/shm, which is not 64 MB
		}
		memVols := memoryBackedVolumes(ps)
		for _, c := range ps.Containers {
			if len(gpuResources(c)) == 0 {
				continue
			}
			if mountsShm(c, memVols) {
				continue
			}
			findings = append(findings, Finding{
				RuleID:   r.ID(),
				Severity: r.Severity(),
				Message: fmt.Sprintf("GPU container %q has no memory-backed /dev/shm mount; Kubernetes caps /dev/shm at 64 MB by default, "+
					"and shared-memory users — PyTorch DataLoader workers, NCCL, Triton's Python backend — "+
					"crash or stall with cryptic errors the first time it fills", c.Name),
				ObjectRef: obj.Ref(),
				Detail:    c.Name + "/dev-shm",
				Source:    obj.Source,
				Fix: "add a volume {emptyDir: {medium: Memory, sizeLimit: <e.g. 8Gi>}} and mount it at /dev/shm " +
					"(the sizeLimit counts against the pod's memory); if this runtime never uses shared memory " +
					"(e.g. llama.cpp-based servers), baseline this finding",
			})
		}
	}
	return findings
}

// memoryBackedVolumes returns the names of pod volumes that are emptyDirs
// with medium Memory — the only kind that actually enlarges /dev/shm.
func memoryBackedVolumes(ps parser.PodSpec) map[string]bool {
	out := map[string]bool{}
	vols, _ := ps.Raw["volumes"].([]any)
	for _, v := range vols {
		vol, ok := v.(map[string]any)
		if !ok {
			continue
		}
		ed, ok := vol["emptyDir"].(map[string]any)
		if !ok {
			continue
		}
		if medium, _ := ed["medium"].(string); medium != "Memory" {
			continue
		}
		if name, ok := vol["name"].(string); ok {
			out[name] = true
		}
	}
	return out
}

// mountsShm reports whether the container mounts one of the memory-backed
// volumes at /dev/shm.
func mountsShm(c parser.Container, memVols map[string]bool) bool {
	mounts, _ := c.Raw["volumeMounts"].([]any)
	for _, m := range mounts {
		mount, ok := m.(map[string]any)
		if !ok {
			continue
		}
		path, _ := mount["mountPath"].(string)
		if strings.TrimSuffix(path, "/") != "/dev/shm" {
			continue
		}
		if name, _ := mount["name"].(string); memVols[name] {
			return true
		}
	}
	return false
}
