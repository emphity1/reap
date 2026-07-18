package rules

import "strings"

// modelServerImagePatterns identify well-known inference servers by image
// name substring, lowercased. Curated for precision over recall: a pattern
// belongs here only if its presence in an image name is unambiguous — a
// false positive costs the tool more credibility than a false negative.
// Ray Serve is absent on purpose: it ships in the generic rayproject/ray
// image that training jobs use too.
//
// Shared detection: model-server-no-probes uses this today; the set-level
// inference-no-hpa and no-pdb rules will reuse it.
var modelServerImagePatterns = []string{
	"vllm",
	"tritonserver",
	"text-generation-inference",
	"torchserve",
	"sglang",
	"ollama",
	"kserve/",
	"nvcr.io/nim",
	"lmdeploy",
}

// IsModelServerImage reports whether the image runs a known inference
// server, and which pattern matched (for use in finding messages).
func IsModelServerImage(image string) (bool, string) {
	img := strings.ToLower(image)
	for _, p := range modelServerImagePatterns {
		if strings.Contains(img, p) {
			return true, p
		}
	}
	return false, ""
}
