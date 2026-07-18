package rules

import (
	"fmt"

	"github.com/emphity1/reap/internal/parser"
)

// JobNoDeadline flags GPU Jobs (and CronJobs) that set no
// activeDeadlineSeconds. A job that hangs — bad data, a NCCL stall, a crash
// loop — keeps its GPUs allocated indefinitely while doing no work; a
// deadline turns that into a bounded loss. CPU-only jobs are out of scope:
// this is a GPU linter, not a general one.
type JobNoDeadline struct{}

func (JobNoDeadline) ID() string { return "job-no-deadline" }

func (JobNoDeadline) Severity() Severity { return Warning }

func (r JobNoDeadline) Check(obj parser.Object) []Finding {
	var deadlinePaths [][]string
	var msgFormat, fix string
	switch obj.Kind {
	case "Job":
		deadlinePaths = [][]string{
			{"spec", "activeDeadlineSeconds"},
			{"spec", "template", "spec", "activeDeadlineSeconds"},
		}
		msgFormat = "Job %q holds %s with no activeDeadlineSeconds; " +
			"if a run hangs, those GPUs stay allocated — nights and weekends included — while doing no work"
		fix = "set spec.activeDeadlineSeconds to a hard upper bound for the run (e.g. twice the expected duration)"
	case "CronJob":
		deadlinePaths = [][]string{
			{"spec", "jobTemplate", "spec", "activeDeadlineSeconds"},
			{"spec", "jobTemplate", "spec", "template", "spec", "activeDeadlineSeconds"},
		}
		msgFormat = "CronJob %q spawns GPU Jobs (%s) with no activeDeadlineSeconds; " +
			"one hung run is enough to keep a GPU allocated until someone notices"
		fix = "set spec.jobTemplate.spec.activeDeadlineSeconds to a hard upper bound for each run"
	default:
		return nil
	}
	for _, path := range deadlinePaths {
		if _, ok := obj.Lookup(path...); ok {
			return nil
		}
	}
	gpus := gpuSummary(obj.Containers())
	if gpus == "" {
		return nil
	}
	return []Finding{{
		RuleID:    r.ID(),
		Severity:  r.Severity(),
		Message:   fmt.Sprintf(msgFormat, obj.Name, gpus),
		ObjectRef: obj.Ref(),
		Source:    obj.Source,
		Fix:       fix,
	}}
}
