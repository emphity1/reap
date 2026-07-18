# CLAUDE.md

Guidance for AI agents (and humans) working in this repository.

## What `reap` is

`reap` is a command-line tool that scans Kubernetes manifests for **GPU waste and
ML/GPU workload misconfigurations** — idle GPUs, bad scheduling, missing
production-readiness — and reports them, in your terminal and in CI, before they
cost you money. Think "kube-linter, specialized for GPU and ML workloads."

**Long-term vision (context only):** this CLI is the free foundation of a broader
GPU FinOps platform. It grows in stages toward live cluster analysis, real GPU
utilization metrics, cost reporting, and policy enforcement. The full roadmap
lives outside this repo. **You are working on the current phase only: the
open-source CLI engine.** Don't build ahead of it.

## CRITICAL: the open-core boundary (read this first)

This repository is the **free, public, open-source core**, licensed under
**Apache 2.0**. Everything here is meant to be freely used and adopted.

**The commercial product lives in a separate, private repository.** It is NOT
part of this codebase. Never add paid/commercial features here. Specifically, do
**not** implement in this repo:

- A web UI / dashboard / frontend of any kind.
- A cost backend, historical database, or time-series storage of spend.
- An admission controller / enforcement webhook meant as an enterprise feature.
- SSO, RBAC, multi-tenancy, licensing, billing, or usage metering.
- Cloud pricing-API integrations or any per-team "showback/chargeback" reporting.

If a request seems to cross this line — anything whose value exists only at
company scale — **stop and ask** before writing code. The rule of thumb: *would a
single developer, running this locally, use it?* If yes, it belongs here. If it
only makes sense with a big GPU cluster and many teams, it does not.

## Core principles (non-negotiable)

1. **Offline-first. No network calls in the core, ever.** `reap` reads local
   files and prints results. It must run with no internet, no credentials, no
   cluster access. This is what makes it frictionless to adopt (no security
   review needed) and testable without hardware. Do not add HTTP clients,
   downloads, or update checks to the core.
2. **No telemetry / no phone-home.** The tool never sends usage data anywhere.
   Privacy is part of the product's trust story. Do not add analytics.
3. **Cloud-agnostic.** `reap` speaks Kubernetes, not AWS/GCP/Azure. A pod that
   requests a GPU looks the same on every platform, so rules are written once and
   work everywhere. Do not add cloud provider SDKs.
4. **Speak in impact.** When a finding can be tied to a concrete cost, say so in
   plain terms (e.g. "an idle A100 in a weekend-running notebook ≈ real money").
   Findings should motivate action, not just flag pedantry.
5. **Minimal dependencies.** Prefer the standard library and a small, trusted set
   of libraries. Every dependency is a liability. Justify new ones.

## Tech stack

- **Language: Go** (1.23+). Single static binary, native Kubernetes libraries,
  the language of this ecosystem (kube-linter, Trivy, kubescape).
- **Rules: native Go for now.** Each rule is a small Go type implementing a
  common `Rule` interface. Keep the interface clean so a *declarative* rule
  loader (Rego/OPA or CEL) can be added in a later phase **without refactoring**.
  Do not build the Rego/CEL system yet — validate the rule *content* first.
- **Packaging: Docker** (distroless image) and prebuilt binaries.
- **YAML:** parse multi-document manifests and the output of `helm template` /
  `kustomize build` piped via stdin.

## Suggested project layout

```
reap/
├── cmd/reap/            # CLI entrypoint (flag parsing, wiring)
├── internal/
│   ├── parser/          # YAML → normalized Kubernetes objects
│   ├── rules/           # Rule interface + one file per rule
│   ├── engine/          # runs all rules over all objects, collects findings
│   └── report/          # output formatters: text, json (sarif later)
├── testdata/            # example manifests: passing + failing fixtures per rule
├── docs/
├── .github/workflows/   # CI (build, test, lint)
├── Makefile
├── go.mod               # module path: github.com/<YOUR_ORG>/reap  ← replace
├── README.md
├── LICENSE              # Apache 2.0
├── CONTRIBUTING.md
└── CLAUDE.md
```

### Rule shape (sketch, adapt as needed)

A rule exposes an ID, a severity, and a check that returns findings:

```go
type Severity int // Info, Warning, Error

type Finding struct {
    RuleID   string
    Severity Severity
    Message  string   // human-readable, actionable, cost-aware when possible
    ObjectRef string  // kind/namespace/name of the offending object
    Fix      string   // optional suggested remediation
}

type Rule interface {
    ID() string
    Severity() Severity
    Check(obj Object) []Finding
}
```

## The initial rule set (current scope of work)

Implement these as the first batch. Each rule ships with passing and failing
fixtures in `testdata/`.

1. Whole GPU requested for a fraction-of-a-GPU workload → suggest MIG/time-slicing.
2. No GPU `limit` set → a pod can monopolize a card.
3. Job with no `activeDeadlineSeconds` → a runaway job burns GPU forever.
4. Distributed training (PyTorchJob/MPIJob) without gang scheduling → deadlock risk.
5. Jupyter/notebook workload without idle-culling → GPU idle nights and weekends.
6. Model server without liveness/readiness probes → traffic to dead pods.
7. Inference workload without an HPA → over/under-provisioning.
8. Critical service without a PodDisruptionBudget → downtime during drains.
9. Multi-node training without topology-aware affinity → NCCL on the slow network.
10. GPU node pool without scale-to-zero configured → idle nodes left running.

## Output & exit codes

- **Human-readable** text by default; **JSON** via a flag (for automation).
  SARIF output is a planned enhancement (for GitHub code scanning) — leave a clean
  seam in the `report` package for it, but don't implement it yet.
- **Severity levels** map to **exit codes** so CI can gate: e.g. `0` clean,
  non-zero when findings at/above a configurable threshold are present.
- Support a **baseline/ignore** mechanism so the tool can be adopted on existing
  repos without drowning users in pre-existing warnings.

## Commands

Establish these in the `Makefile` from the start:

- `make build` — build the `reap` binary.
- `make test` — run `go test ./...`.
- `make lint` — run `gofmt`/`go vet` (and `golangci-lint` if configured).
- `reap ./manifests/` or `helm template . | reap -` — run the linter.

## Testing without GPU hardware

The whole tool is designed to be validated small but correct at scale:

- **Unit tests** use fixtures in `testdata/` (a passing and a failing manifest per
  rule). This is the primary test method for the CLI.
- **At-scale validation** (for later phases) uses *public production traces* —
  e.g. `alibaba/clusterdata` (GPU traces up to ~155k GPUs), Microsoft Philly,
  Azure packing traces — replayed through the rule logic. No hardware needed.
- **Live-mode testing** (a later phase) uses the fake-gpu-operator to make a local
  `kind` cluster report GPUs it doesn't have.

## Licensing & contributions

- License: **Apache 2.0** (see `LICENSE`). Keep every source file compatible.
- Contributions require **DCO sign-off**: commit with `git commit -s`. This keeps
  provenance clean so the project stays legally healthy. Reflect this in
  `CONTRIBUTING.md`.

## How to work in this repo (agent behavior)

- **One vertical slice first.** A single rule working end-to-end (parse → check →
  report → passing test) is worth more than ten stubbed rules. Build the skeleton
  and prove the pipeline, then add rules one by one.
- **Small, focused changes.** Prefer incremental commits over sweeping rewrites.
- **Tests with every rule.** No rule lands without fixtures and a test.
- **Update docs as you go.** New flags or behavior → update the README.
- **When the open/closed boundary is unclear, ask.** Do not implement anything
  from the "do not build here" list on your own initiative.
- **Explain trade-offs, don't just execute.** If there's a meaningfully better
  approach than what was asked, say so before proceeding.