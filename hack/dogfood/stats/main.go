// Command stats prints corpus statistics for the dogfood run: object,
// pod-spec, container, and GPU-requesting-container counts per input file.
// It reuses reap's own parser so the counts share the tool's semantics
// (same multi-doc handling, same GPU resource detection).
package main

import (
	"fmt"
	"os"

	"github.com/emphity1/reap/internal/parser"
	"github.com/emphity1/reap/internal/rules"
)

func main() {
	objs, err := parser.Load(os.Args[1:], os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stats: %v\n", err)
		os.Exit(2)
	}
	var podSpecs, containers, gpuContainers int
	kinds := map[string]int{}
	for _, o := range objs {
		kinds[o.Kind]++
		specs := o.PodSpecs()
		podSpecs += len(specs)
		for _, s := range specs {
			for _, c := range s.Containers {
				containers++
				gpu := false
				for res := range c.Requests {
					if rules.IsGPUResource(res) {
						gpu = true
					}
				}
				for res := range c.Limits {
					if rules.IsGPUResource(res) {
						gpu = true
					}
				}
				if gpu {
					gpuContainers++
				}
			}
		}
	}
	fmt.Printf("objects\t%d\n", len(objs))
	fmt.Printf("podSpecs\t%d\n", podSpecs)
	fmt.Printf("containers\t%d\n", containers)
	fmt.Printf("gpuContainers\t%d\n", gpuContainers)
}
