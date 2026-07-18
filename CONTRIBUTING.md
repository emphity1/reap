# Contributing to reap

Thanks for helping make GPU workloads on Kubernetes less wasteful.

## Ground rules

- **Offline-first.** The core makes no network calls, ever — no HTTP clients,
  no downloads, no update checks, no telemetry.
- **Cloud-agnostic.** Rules speak Kubernetes, not AWS/GCP/Azure.
- **Minimal dependencies.** Prefer the standard library; justify every new
  dependency in your PR description.

## Developer Certificate of Origin (DCO)

Every commit must be signed off, certifying that you have the right to submit
the work under the project license ([Apache 2.0](LICENSE)):

```sh
git commit -s
```

This adds a `Signed-off-by:` line to the commit message. PRs with unsigned
commits cannot be merged.

## Adding a rule

1. Create `internal/rules/<rule_name>.go` implementing the `Rule` interface
   (see `internal/rules/rule.go`), and register it in `All()`.
2. Add fixtures under `testdata/<rule-id>/`: `good.yaml` (passes) and
   `bad.yaml` (fails).
3. Add a table-driven test next to the rule. No rule lands without fixtures
   and a test.
4. Write findings that motivate action: say what it costs and how to fix it,
   not just what is wrong.
5. Update the rules table in the README.

## Before opening a PR

```sh
make lint
make test
```
