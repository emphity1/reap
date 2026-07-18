# reap

Find and cut wasted GPU spend in Kubernetes. Static checks for ML/GPU workloads, from your CI to your cluster.

`reap` scans Kubernetes manifests for GPU waste and ML workload
misconfigurations — idle GPUs, bad scheduling, missing production-readiness —
and reports them in your terminal and in CI, before they cost you money.
Think "kube-linter, specialized for GPU and ML workloads."

It reads local files only: no network, no credentials, no cluster access,
no telemetry.

## Install

```sh
git clone https://github.com/emphity1/reap.git
cd reap
make build   # produces bin/reap
```

## Usage

```sh
# lint a file or a directory (scanned recursively for *.yaml / *.yml)
reap ./manifests/

# lint rendered charts / overlays via stdin
helm template . | reap -
kustomize build . | reap -
```

Example output:

```
testdata/no-gpu-limit/bad.yaml: Deployment/ml/llm-inference
  [error] no-gpu-limit: container "server" requests nvidia.com/gpu: 1 but sets no limit; Kubernetes rejects GPU requests without an equal limit, so this manifest will not deploy
        fix: set resources.limits["nvidia.com/gpu"] equal to the request

reap: 2 object(s) checked, 1 finding(s): 1 error, 0 warning, 0 info
```

### Exit codes

| Code | Meaning                                          |
|------|--------------------------------------------------|
| 0    | no findings at/above the threshold               |
| 1    | findings at/above the threshold (CI should fail) |
| 2    | usage or input error                             |

The threshold is set with `-fail-on` (`info`, `warning`, `error`, or `none`
to always exit 0); the default is `warning`.

### JSON output

`-format json` emits a machine-readable report (schema version `1`):

```json
{
  "version": "1",
  "summary": {
    "objectsChecked": 2,
    "findings": 1,
    "bySeverity": {"error": 1, "warning": 0, "info": 0}
  },
  "findings": [
    {
      "ruleId": "no-gpu-limit",
      "severity": "error",
      "message": "container \"server\" requests nvidia.com/gpu: 1 but sets no limit; ...",
      "object": "Deployment/ml/llm-inference",
      "detail": "server/nvidia.com/gpu",
      "source": "manifests/app.yaml",
      "fix": "set resources.limits[\"nvidia.com/gpu\"] equal to the request",
      "fingerprint": "c3c88f7fa45669b8"
    }
  ]
}
```

`fingerprint` is the finding's **stable identity**: a hash of the rule ID, the
object reference (`Kind/Namespace/Name`), and a semantic detail key (container
and resource names — never positional indices). Message wording, severity,
suggested fix, and **file path are metadata and never enter the hash**, so the
same chart produces the same fingerprint whether linted from a file or piped
through stdin, and fingerprints survive copy edits and file moves. Identical
logical findings from different files (e.g. two overlays defining the same
object with the same violation) deliberately share one fingerprint. The
upcoming baseline/ignore mechanism is a set of these fingerprints.

## Rules

| ID                | Severity | Checks                                                                 |
|-------------------|----------|------------------------------------------------------------------------|
| `no-gpu-limit`    | error    | a container requests a GPU without an equal limit (rejected by the API server; the request/limit pair must be equal for extended resources) |
| `job-no-deadline` | warning  | a GPU `Job` or `CronJob` sets no `activeDeadlineSeconds`, so a hung run holds its GPUs indefinitely |

More GPU-waste rules (idle notebooks, missing gang scheduling, missing
probes/HPA/PDB, topology-unaware training, GPU node pools without
scale-to-zero) are planned — each ships with passing and failing fixtures
in `testdata/`.

## Development

```sh
make build   # build bin/reap
make test    # go test ./...
make lint    # gofmt + go vet
```

See [CONTRIBUTING.md](CONTRIBUTING.md). Licensed under [Apache 2.0](LICENSE).
