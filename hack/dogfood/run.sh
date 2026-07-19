#!/usr/bin/env bash
# Run reap over every rendered corpus file and collect machine-readable
# results into _dogfood/results/. Offline: reads only what fetch.sh produced.
#
# Per input, writes:
#   results/<corpus>/<name>[.variant].json  reap -format json output
#   results/<corpus>/<name>[.variant].meta  runtime, exit code, stderr, size,
#                                           object/container/GPU counts (via
#                                           hack/dogfood/stats)
set -euo pipefail

cd "$(dirname "$0")/../.."
DOG=_dogfood
BIN=bin/reap
[[ -x "$BIN" ]] || { echo "build first: make build" >&2; exit 1; }

now_ms() { perl -MTime::HiRes=time -e 'printf "%d", time()*1000'; }

run_one() { # corpus label file
  local corpus="$1" label="$2" file="$3"
  local outdir="$DOG/results/$corpus"
  mkdir -p "$outdir"
  local t0 t1 code
  t0=$(now_ms)
  set +e
  "$BIN" -format json -fail-on none "$file" \
    > "$outdir/$label.json" 2> "$outdir/$label.stderr.txt"
  code=$?
  set -e
  t1=$(now_ms)
  {
    printf 'file\t%s\n'    "$file"
    printf 'exit\t%s\n'    "$code"
    printf 'ms\t%s\n'      "$((t1 - t0))"
    printf 'bytes\t%s\n'   "$(wc -c < "$file" | tr -d ' ')"
    go run ./hack/dogfood/stats "$file" 2>/dev/null || printf 'stats\tFAILED\n'
  } > "$outdir/$label.meta"
  [[ -s "$outdir/$label.stderr.txt" ]] || rm -f "$outdir/$label.stderr.txt"
}

for dir in "$DOG"/rendered/*/; do
  name=$(basename "$dir")
  for f in "$dir"*.yaml; do
    [[ -e "$f" ]] || continue
    variant=$(basename "$f" .yaml)
    run_one charts "$name.$variant" "$f"
  done
done

for f in "$DOG"/rendered-examples/*.yaml; do
  [[ -e "$f" ]] || continue
  run_one examples "$(basename "$f" .yaml)" "$f"
done

# Derived detector probes: GPU variants of upstream examples (committed in
# hack/dogfood/variants/, each header documents its base and delta). Reported
# separately from the pristine corpora.
for f in hack/dogfood/variants/*.yaml; do
  [[ -e "$f" ]] || continue
  run_one variants "$(basename "$f" .yaml)" "$f"
done

echo "results in $DOG/results/" >&2
