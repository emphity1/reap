# Dogfood report — reap v0.1.0 against real-world manifests

Run date: 2026-07-19. Tool: reap v0.1.0 (commit 67070a8), all 10 rules.
Method: see `dogfood` plan; corpus pinned in `charts.tsv` / `examples.tsv`,
reproduced by `fetch.sh` + `run.sh`. Raw results in `_dogfood/results/`
(gitignored; regenerate with the scripts).

## Headline numbers

| Metric | Value |
|---|---|
| Inputs linted | 27 (15 chart renders from 12 charts + 12 example manifests) |
| Objects parsed | 278 |
| Crashes / parse errors / stderr output | **0 / 0 / 0** |
| Runtime per input (warm) | 7–47 ms (a 693 ms outlier was process cold-start, not parsing) |
| Total findings | 26 (7 warning, 19 info; 0 error) |
| Chart default renders with zero findings | **9 of 12** |
| Example manifests with zero findings | 9 of 12 |
| Median findings per chart (default values) | **0** |
| Max findings on one input | 7 (vLLM docs page — two same-named Deployments double 3 of them) |
| Confirmed false positives | **1 pattern** (KServe controller flagged as a model server) |
| Confirmed parser blindness | **1 bug** (pod templates nested in arrays are invisible) |

The headline shape is right: quiet on well-written charts, and the findings on
GPU inputs are few and mostly defensible. The two confirmed bugs below are the
real output of this run.

## Corpus

**A — vendor charts** (pinned; `default` = stock values, `gpu` = override in
`values/`): ray-cluster 1.6.2 (default+gpu), jupyterhub 4.4.0, gpu-operator
v26.3.3, vllm-stack 0.1.11 (default+gpu), volcano 1.15.0, kueue 0.18.3,
ollama 1.67.0 (default+gpu), dcgm-exporter 4.8.3, mlflow 1.11.2,
seldon-core-operator 1.19.0, kserve-resources v0.19.0, triton k8s-onprem
@ server v2.59.0 (chart deps stripped to render; its default already requests
a GPU). GPU renders were verified to actually contain a GPU resource.
jupyterhub and the operator charts have no meaningful `gpu` variant: their
GPU-bearing pods are created at runtime, not rendered.

**B — example manifests** (pinned to tags/SHAs): PyTorchJob simple
(training-operator v1.8.1 and kubeflow/trainer v1.9.2 — note: the trainer
example is still a v1 PyTorchJob, so Trainer-v2 `TrainJob` remains untested),
MPIJob tensorflow-benchmarks (GPU), RayJob sample + RayJob batch-inference
(GPU), Kueue sample-job + sample-pytorchjob, Volcano dist-mnist (a Volcano
`Job`, not a TFJob as planned), KServe sklearn + torchserve-gpu
InferenceServices, the vLLM k8s deployment docs page (NVIDIA + AMD
Deployments), Kubeflow Notebook sample.

## Per-rule results

"Combos" = distinct (rule, input) pairs; instance counts differ where an input
repeats an object (ollama has two renders; the vLLM docs page defines the same
Deployment twice).

| Rule | Combos | Instances | Verdict mix |
|---|---|---|---|
| `shm-too-small` | 5 | 5 | 2 TP (MPIJob, RayJob batch-inference), 3 debatable (ollama, triton, vllm-stack — re-judged, see the verification addendum) |
| `gpu-no-node-targeting` | 4 | 5 | 4 debatable (portable-by-intent examples); correctly suppressed on RayJob (has nodeSelector) |
| `no-pdb` | 5 | 7 | **1 FP (kserve controller)**, 4 TP at info level |
| `inference-no-hpa` | 4 | 6 | **1 FP (kserve controller)**, 3 TP at info level; correctly suppressed on Triton (HPA in render) |
| `model-server-no-probes` | 1 | 1 | 1 TP — the AMD/ROCm Deployment in vLLM's own docs ships without probes |
| `distributed-training-no-gang-scheduling` | 1 | 1 | 1 TP — MPIJob, 2 GPU workers, no gang evidence |
| `multinode-no-topology-affinity` | 1 | 1 | 1 TP (info) — same MPIJob |
| `no-gpu-limit` | 0 | 0 | no such mistake in corpus; correctly stayed silent on Triton's limits-only GPU (valid in K8s) |
| `job-no-deadline` | 0 | 0 | vacuous — no `batch/v1` GPU Job in corpus |
| `notebook-no-idle-culling` | 0 | 0 | vacuous — no GPU notebook in corpus (rule is GPU-gated) |

Notable true positive worth telling users about: **the AMD example in vLLM's
docs has no probes** while the NVIDIA one does. (An earlier draft of this
report also claimed the production-stack chart "does not mount /dev/shm";
upstream template inspection later showed the mount exists but is gated on
the chart's own `tensorParallelSize` knob — see the verification addendum.
Corrected rather than deleted, because the correction is itself a finding
about the rule's breadth.)

## Oracle results

| Oracle | Result |
|---|---|
| z2jh ships culling enabled (verified: `cull.enabled: true`) → rule must not fire on default render | PASS, but **vacuous**: no rendered pod has a GPU, so the GPU gate silences the rule before the culling detector is ever consulted |
| Renders that configure `/dev/shm` must not trip `shm-too-small` | **PASS, genuine**: both vLLM docs Deployments mount memory-backed shm and were not flagged; all 5 firings were on inputs with no shm mount |
| Kueue/Volcano-managed training must not trip the gang rule | **vacuous**: the upstream samples are CPU-only (GPU gate), and the Volcano example is a Volcano `Job`, which reap cannot see into at all (parser bug below) |
| HPA/PDB present in the same render must suppress `inference-no-hpa` / `no-pdb` | **PASS, genuine**: Triton's render includes an HPA targeting its Deployment — `inference-no-hpa` stayed silent while `no-pdb` (genuinely absent) fired |

One oracle-class failure the plan didn't list: the **KServe controller FP**
(below) is exactly the "loud wrong on good manifests" case.

## Bugs found (fix before release)

1. **Parser: pod templates nested in arrays are invisible.**
   `findPodSpecs` recurses into maps only, never into `[]any`
   (`internal/parser/object.go:143-147`). Consequences observed:
   the GPU worker in ray-cluster's `workerGroupSpecs[]` produced **0 findings
   and 0 visible GPU containers** on the `gpu` render (fetch.sh's grep proved
   the GPU is there), and the Volcano `Job`'s `tasks[]` showed 0 containers.
   Reap is blind to the GPU-bearing half of every Ray manifest — the #1 chart
   on the plan's own priority list.
2. **Model-server detector: `"kserve/"` matches the KServe control plane.**
   `kserve/kserve-controller:v0.19.0` is flagged as a model server, so the
   controller-manager gets `inference-no-hpa` + `no-pdb` on a stock render.
   Confirmed FP. The pattern was meant for serving runtimes
   (`kserve/sklearnserver` etc.), not the operator; `kserve/storage-initializer`
   would be next.
3. **Latent, coupled to fix #1: `job-no-deadline` matches on `Kind == "Job"`
   only.** Volcano's `Job` (`batch.volcano.sh/v1alpha1`) has no
   `spec.activeDeadlineSeconds`; today its pods are invisible, but the moment
   array descent is fixed, every GPU Volcano Job becomes a false positive.
   The kind match must check the `batch/v1` group when arrays become visible.

## Design points confirmed by the run

- **Fingerprint dedup works as documented**: the vLLM docs page defines two
  same-named Deployments; 7 findings collapse to 4 unique fingerprints, and
  baselining suppresses both copies.
- **Baseline flow is flawless end-to-end** on a noisy render:
  `-write-baseline` (4 entries) → clean run, exit 0 → one injected violation →
  exactly that one finding, exit 1.
- **Severity discipline holds**: a first run on the noisiest real chart shows
  1 warning + 3 info, each with a fix and explicit "baseline this if external"
  advice. No wall of noise. Info-level set rules dominated the count (19/26),
  which is what their design predicted.
- **`no-gpu-limit` semantics are right**: Triton's limits-only GPU (valid for
  extended resources) was not flagged.

## Recommended changes, prioritized

1. **P0 — recurse into arrays in `findPodSpecs`** + RayCluster/RayJob GPU
   worker fixtures. Unlocks Ray and Volcano. Do #3's apiVersion guard in the
   same change, with a Volcano Job fixture.
2. **P0 — narrow the model-server detector**: replace `"kserve/"` with the
   explicit serving-runtime images, or exclude images matching
   `controller|manager|operator|initializer`. Add the kserve-resources render
   pattern as a regression fixture.
3. **P1 — InferenceService visibility**: `spec.predictor.<framework>` /
   `spec.predictor.model` carry resources but no `containers` array, so KServe
   workloads (torchserve-gpu example: 0 containers seen) are invisible. Either
   synthesize a pod-spec view for InferenceService or document the gap; an
   inference linter that skips KServe's CRD is hard to explain.
4. **P1 — decide `shm-too-small` breadth**: 2 of 5 firings (ollama/llama.cpp,
   triton) are debatable — those runtimes don't lean on PyTorch DataLoader/NCCL
   shm the way the rule's message claims. Options: keep broad (fix is cheap and
   harmless) or gate the *warning* severity on torch/vllm/ray-family images and
   report info otherwise. Leaning: keep broad, soften the message.
5. **P2 — order findings by severity within an object** in text output: on the
   vllm-stack first run the one warning prints after three info findings.
6. **P2 — fix `wrote %d baseline entr(y/ies)`** in `cmd/reap/main.go` to use
   proper pluralization.
7. **Corpus gaps to close next run** (rules that stayed vacuously silent):
   a GPU notebook manifest, a GPU PyTorchJob with a Kueue queue label (makes
   the gang oracle real), a `batch/v1` GPU Job, and a real Trainer-v2
   `TrainJob`.

Nothing else changed during the run: all verdicts above were gathered first,
per the rules of engagement.

---

# Post-fix re-run (2026-07-19, same corpus + gap probes)

Fixes applied after the gather phase, each TDD'd with a red test first:

1. `findPodSpecs` now recurses into arrays, and the core-kind pod-spec paths
   are gated on the API group (a `batch.volcano.sh` Job is not a `batch/v1`
   Job). Regression fixtures: RayCluster with an array-nested GPU worker
   (must fire `shm-too-small`), Volcano Job unit tests.
2. `job-no-deadline` requires API group `batch` — the latent Volcano false
   positive predicted above became real the moment arrays turned visible,
   and is now guarded.
3. The bare `"kserve/"` detector pattern is replaced with KServe's six
   serving-runtime images (verified against upstream
   `config/runtimes/kustomization.yaml`); the control plane no longer
   matches. Regression fixture: a probe-less kserve-controller Deployment
   that must produce zero findings, asserted via a new absence check in the
   e2e harness.

A kind-only audit of every rule found no further live collisions: the
training-kind rules self-guard (they require the matching `*ReplicaSpecs`
key), and the HPA/PDB index works in the suppression direction, where a
wrong match costs a false negative, not credibility.

## Numbers after the fixes

| Input | Before | After | Why |
|---|---|---|---|
| ray-cluster.gpu | 0 findings, GPU invisible | 2 (shm-too-small, gpu-no-node-targeting) | array descent — both are true positives; the KubeRay chart really ships no shm mount, closing the Ray oracle **genuinely**: the chart does *not* configure `/dev/shm`, so firing is correct |
| kserve-resources.default | 2 (both FP) | **0** | detector narrowed |
| volcano-tfjob-dist-mnist | 0 (blind: 0 containers) | 0 (2 containers seen) | silent for the right reason now — group guard, plus the example is CPU-only |

Corpus totals moved from 26 findings (2 of them FPs) to 26 findings
(0 confirmed FPs): the two lost KServe FPs are replaced by the two genuine
Ray findings the parser used to miss. Confirmed-FP count: **1 pattern → 0**.

## Corpus gaps closed

New pristine inputs (pinned): Kubeflow **TrainJob v2** examples
(multi-node + Kueue integration) and the **AKS GPU tutorial Job**
(`samples-tf-mnist-demo` from `use-nvidia-gpu.md` — the manifest thousands of
users copy). New **derived probes** in `hack/dogfood/variants/` (each header
documents its upstream base and exact delta; reported separately from the
pristine corpora):

| Probe | Expectation | Result |
|---|---|---|
| PyTorchJob GPU + Kueue queue label | gang rule silent | **PASS** |
| same, label removed (control) | gang rule fires | **PASS** — proves the suppression test is not vacuous |
| GPU notebook, no culling | notebook rule fires | **PASS** |
| GPU notebook + Jupyter's real `shutdown_no_activity_timeout` flag (control) | notebook rule silent | **PASS** |

`job-no-deadline` also stopped being vacuous: it fires on the AKS tutorial
Job (no `activeDeadlineSeconds` — a true positive worth a README example),
alongside a debatable-to-TP `shm-too-small`. Every rule except `no-gpu-limit`
has now fired at least once on real or probe input, and `no-gpu-limit`'s
silence is the correct behavior for manifests that set limits properly
(validated on Triton's limits-only pattern).

**New blindness, documented:** TrainJob v2 carries no pod template at all —
it lives in the referenced `ClusterTrainingRuntime`. Both TrainJob examples
parse (1 object) but expose 0 containers. Same problem class as
InferenceService: recorded as the P1 "CRD visibility" work item, not
fixable by parser recursion.

## Follow-ups — all resolved (2026-07-19)

- **InferenceService / TrainJob v2 visibility → documented boundary, not
  code.** Pod-level findings on operator-managed CRDs would be unfixable or
  wrong (operators inject probes and autoscale; TrainJob's pod template
  lives in the referenced ClusterTrainingRuntime). The README now explains
  this under "deliberately not checked", including the workaround: lint the
  rendered runtime output via stdin.
- **`shm-too-small` breadth → stays broad.** The message stops overclaiming
  PyTorch (it now names NCCL and Triton's Python backend — Triton was
  re-judged TP-leaning on that basis) and the fix names the baseline escape
  hatch for runtimes that never use shared memory (ollama; vllm-stack later
  joined the debatable set, see the verification addendum).
- **Severity ordering** — findings sort error → warning → info within an
  object (engine-level, shared by text and JSON).
- **Pluralization** of the write-baseline message fixed.

---

# Verification addendum (post-v0.2.0)

## Parser fix verified by the denominator, not the findings count

The 26 → 26 findings total was consistent with the fixes but proves little
by itself. The number that demonstrates the array recursion works in
general is what the parser *sees*. Old parser (pre-fix commit) vs v0.2.0,
same rendered corpus:

| Corpus total | before | after |
|---|---|---|
| pod specs | 54 | 59 |
| containers | 59 | 64 |
| GPU containers | 14 | 15 |

Exactly four inputs changed — precisely the four with array-nested
templates, covering both structural families of the fix: ray-cluster
default (1→2 pod specs, CPU worker group) and gpu (1→2, **0→1 GPU
containers** — the original blind spot), rayjob-sample (1→2), and the
Volcano Job (0→2: it previously took the wrong typed path via the kind
collision). No map-nested input changed (MPIJob, PyTorchJob unchanged —
no regression), and no array-nested input in the corpus stayed invisible.

## vllm-stack `shm-too-small` verdict revised: TP → debatable

Preparing an upstream patch, template inspection showed the chart (already
at tag 0.1.11) mounts `/dev/shm` **iff `vllmConfig.tensorParallelSize` is
set** — semantically aligned with vLLM's own docs ("vLLM needs to access
the host's shared memory for tensor parallel inference"). Our minimal GPU
render set no `vllmConfig`, so the mount was absent and reap fired. The
chart's gating is more nuanced than reap's deliberately broad rule; the
finding on that render is debatable, not a chart bug. **No upstream PR** —
it would have been wrong. Lesson recorded: verify the upstream *condition*,
not just the rendered absence, before claiming a true positive.

## Release v0.2.0 verified

- `reap_0.2.0_darwin_arm64.tar.gz`: checksum matches `checksums.txt`,
  binary runs, correct findings and exit code on fixtures.
- `go install github.com/emphity1/reap/cmd/reap@v0.2.0`: resolves via the
  module proxy, builds, and the installed binary contains the parser fix
  (array-nested RayCluster fixture produces its finding).

## Upstream contribution prepared

vLLM docs (`docs/deployment/k8s.md`, unchanged upstream at the analyzed
SHA): the AMD/ROCm example Deployment lacks the liveness/readiness probes
the NVIDIA example has, on the same `/health:8000` endpoint. A 12-line
patch mirroring the NVIDIA probes verbatim applies cleanly; PR to be
opened from a fork (fix-first framing).

---

# Second verification addendum (pre-launch checks)

## Reproduction from a clean state

`_dogfood/` deleted entirely, `fetch.sh` re-run from zero: all 30 inputs
fetched and rendered with no failures and no GPU-sanity warnings — every
pinned source is still available. On the original 27-input corpus the
fresh numbers are: 27 inputs, 278 objects, 26 findings — identical — but
the severity split is **0 error / 8 warning / 18 info** and clean default
renders are **10 of 12**. The headline table above documents the
*pre-fix* v0.1.0 run (7/19 and 9/12): the fixes removed the two KServe
info FPs and surfaced one warning + one info on the previously-invisible
Ray worker. Per-rule totals reconcile exactly with the deltas recorded in
"Numbers after the fixes" (shm 5→6, node-targeting 5→6, no-pdb 7→6,
inference-no-hpa 6→5). Anyone reproducing today should expect the
post-fix numbers.

## Second retraction: the Ray shm findings are operator-injected — FP

Verifying an upstream-docs citation for the launch post, we read the
KubeRay operator source instead of the docs — and it overturns both Ray
`shm-too-small` findings. `ray-operator/controllers/ray/common/pod.go`
**injects a memory-backed emptyDir at `/dev/shm` into every pod it builds**
from a RayCluster/RayJob: unconditionally at v1.4.2, and at master skipped
only when the user sets an explicit `plasma-directory`:

    // Add /dev/shm volumeMount for the object store to avoid performance degradation.
    // Skip injection when users explicitly set plasma-directory.

So the manifest-level absence reap flags on Ray kinds is present at
system level, exactly like the vllm-stack `tensorParallelSize` case. Both
"true positives" on Ray inputs (ray-cluster.gpu chart render,
`ray-job.batch-inference` sample) are **retracted → system-level FP**.
The `gpu-no-node-targeting` finding on the same render stands: the
operator does not inject scheduling config.

Revised shm-too-small verdict mix: **1 TP (MPIJob — mpi-operator has zero
shm handling, verified), 2 retracted (Ray kinds), 3 debatable (ollama,
triton, vllm-stack)**.

**Action queued before the launch post:** exclude Ray CRD kinds
(RayCluster/RayJob/RayService) from `shm-too-small`, with the same
rationale the gang rule already documents for excluding Ray kinds
(operator-level behavior invisible in the manifest). Until then reap
emits a known FP on every GPU Ray manifest — the exact first-user
experience the launch cannot afford. The parser regression fixture
`testdata/shm-too-small/bad-raycluster.yaml` asserts this very firing and
must move to a different rule (node-targeting) or kind when the rule is
fixed.
