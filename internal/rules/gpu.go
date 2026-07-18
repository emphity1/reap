package rules

import (
	"sort"
	"strconv"
	"strings"

	"github.com/emphity1/reap/internal/parser"
)

// IsGPUResource reports whether a resource name refers to a GPU or a GPU
// slice. The "/gpu" suffix covers the vendor device plugins in common use
// (nvidia.com/gpu, amd.com/gpu, intel.com/gpu); MIG partitions publish their
// own nvidia.com/mig-* resource names.
func IsGPUResource(name string) bool {
	return strings.HasSuffix(name, "/gpu") || strings.HasPrefix(name, "nvidia.com/mig-")
}

// gpuResources returns the GPU resource names a container references in
// requests or limits, sorted for deterministic output.
func gpuResources(c parser.Container) []string {
	set := map[string]bool{}
	for name := range c.Requests {
		if IsGPUResource(name) {
			set[name] = true
		}
	}
	for name := range c.Limits {
		if IsGPUResource(name) {
			set[name] = true
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// gpuSummary renders the GPUs a set of containers asks for, summed per
// resource, e.g. "nvidia.com/gpu: 8". Empty when no container references a
// GPU, which is how rules decide a workload is out of scope.
func gpuSummary(containers []parser.Container) string {
	totals := map[string]int{}
	var names []string
	for _, c := range containers {
		for _, res := range gpuResources(c) {
			if _, seen := totals[res]; !seen {
				names = append(names, res)
			}
			qty := c.Limits[res]
			if qty == "" {
				qty = c.Requests[res]
			}
			n, err := strconv.Atoi(qty)
			if err != nil {
				n = 0 // non-integer quantity: still record the resource name
			}
			totals[res] += n
		}
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, res := range names {
		if totals[res] > 0 {
			parts = append(parts, res+": "+strconv.Itoa(totals[res]))
		} else {
			parts = append(parts, res)
		}
	}
	return strings.Join(parts, ", ")
}
