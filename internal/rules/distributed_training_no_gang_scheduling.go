package rules

import (
	"fmt"

	"github.com/emphity1/reap/internal/parser"
)

// trainingReplicaSpecKeys maps Kubeflow training CRDs to the spec key that
// holds their replica specs. Ray kinds (RayJob/RayCluster) are deliberately
// absent: since KubeRay v1.3 the batch scheduler is configured at the
// operator level and is invisible in the manifest, so a static check would
// systematically misfire there.
var trainingReplicaSpecKeys = map[string]string{
	"PyTorchJob": "pytorchReplicaSpecs",
	"TFJob":      "tfReplicaSpecs",
	"MPIJob":     "mpiReplicaSpecs",
	"XGBoostJob": "xgbReplicaSpecs",
	"PaddleJob":  "paddleReplicaSpecs",
}

// Manifest-visible gang-scheduling evidence. These are evidence lists, not
// detection lists: the asymmetry is deliberate. A key listed here can only
// suppress a finding (worst case: a false negative), so we accept every
// mechanism we know of; the flagging side stays conservative.
// Names verified against upstream docs (Kueue, Volcano, scheduler-plugins,
// KAI, YuniKorn) — see the rule's README entry.
var gangLabels = []string{
	"kueue.x-k8s.io/queue-name",        // Kueue
	"kai.scheduler/queue",              // KAI scheduler
	"scheduling.x-k8s.io/pod-group",    // scheduler-plugins coscheduling
	"pod-group.scheduling.sigs.k8s.io", // coscheduling, pre-1.25 label
	"yunikorn.apache.org/queue",        // Apache YuniKorn
}

var gangAnnotations = []string{
	"scheduling.k8s.io/group-name",           // Volcano PodGroup reference
	"scheduling.volcano.sh/group-name",       // Volcano PodGroup reference
	"scheduling.volcano.sh/group-min-member", // Volcano auto-created PodGroup
	"yunikorn.apache.org/task-group-name",    // YuniKorn gang
	"yunikorn.apache.org/task-groups",        // YuniKorn gang
}

// DistributedTrainingNoGangScheduling flags multi-replica GPU training jobs
// (Kubeflow PyTorchJob, TFJob, MPIJob, XGBoostJob, PaddleJob) with no
// gang-scheduling configuration visible in the manifest. Without gang
// scheduling, replicas that start first hold their GPUs waiting for peers
// that may never be scheduled — a deadlock that burns every GPU it grabbed.
//
// Known limit: with Volcano the training operator can gang-schedule by
// default with nothing in the manifest. If that's your setup, baseline this
// finding — the manifest genuinely carries no evidence either way.
type DistributedTrainingNoGangScheduling struct{}

func (DistributedTrainingNoGangScheduling) ID() string {
	return "distributed-training-no-gang-scheduling"
}

func (DistributedTrainingNoGangScheduling) Severity() Severity { return Warning }

func (r DistributedTrainingNoGangScheduling) Check(obj parser.Object) []Finding {
	specKey, ok := trainingReplicaSpecKeys[obj.Kind]
	if !ok {
		return nil
	}
	rolesVal, ok := obj.Lookup("spec", specKey)
	if !ok {
		return nil
	}
	roles, ok := rolesVal.(map[string]any)
	if !ok {
		return nil
	}
	replicas := totalReplicas(roles)
	if replicas <= 1 {
		return nil
	}
	gpus := gpuSummary(obj.Containers())
	if gpus == "" {
		return nil
	}
	if hasGangEvidence(obj, roles) {
		return nil
	}
	return []Finding{{
		RuleID:   r.ID(),
		Severity: r.Severity(),
		Message: fmt.Sprintf("%s %q runs %d replicas holding GPUs with no gang-scheduling configuration visible in the manifest; "+
			"replicas that start first hold their GPUs waiting for peers that may never be scheduled — a deadlock that burns every GPU it grabbed",
			obj.Kind, obj.Name, replicas),
		ObjectRef: obj.Ref(),
		Source:    obj.Source,
		Fix: "set spec.runPolicy.schedulingPolicy (with a gang scheduler such as Volcano installed), submit through a Kueue queue " +
			"(label kueue.x-k8s.io/queue-name), or set the pod template's schedulerName to your gang scheduler; " +
			"if your training operator gang-schedules by default, baseline this finding",
	}}
}

// totalReplicas sums replicas across roles; a role without an explicit
// replicas field counts as 1, matching the Kubeflow default.
func totalReplicas(roles map[string]any) int {
	total := 0
	for _, roleVal := range roles {
		role, ok := roleVal.(map[string]any)
		if !ok {
			continue
		}
		if n, ok := asInt(role["replicas"]); ok {
			total += n
			continue
		}
		total++
	}
	return total
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

func hasGangEvidence(obj parser.Object, roles map[string]any) bool {
	if _, ok := obj.Lookup("spec", "runPolicy", "schedulingPolicy"); ok {
		return true
	}
	metas := []any{obj.Raw["metadata"]}
	for _, roleVal := range roles {
		role, ok := roleVal.(map[string]any)
		if !ok {
			continue
		}
		tmpl, ok := role["template"].(map[string]any)
		if !ok {
			continue
		}
		metas = append(metas, tmpl["metadata"])
		if spec, ok := tmpl["spec"].(map[string]any); ok {
			if name, ok := spec["schedulerName"].(string); ok && name != "" && name != "default-scheduler" {
				return true
			}
		}
	}
	for _, metaVal := range metas {
		meta, ok := metaVal.(map[string]any)
		if !ok {
			continue
		}
		if hasAnyKey(meta["labels"], gangLabels) || hasAnyKey(meta["annotations"], gangAnnotations) {
			return true
		}
	}
	return false
}

func hasAnyKey(v any, keys []string) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}
