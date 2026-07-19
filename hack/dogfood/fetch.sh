#!/usr/bin/env bash
# Fetch and render the dogfood corpus into _dogfood/rendered/.
#
# Corpus A (charts.tsv): helm charts, rendered per variant with pinned
# versions. Corpus B (examples.tsv): raw example manifests from upstream
# docs, fetched as-is (the markdown entry gets its ```yaml blocks extracted).
#
# Network is used HERE only; everything downstream (run.sh, reap) is offline.
set -euo pipefail

cd "$(dirname "$0")/../.."
DOG=_dogfood
mkdir -p "$DOG/rendered" "$DOG/charts-src"

log() { printf '>>> %s\n' "$*" >&2; }

# --- Corpus A: charts ------------------------------------------------------
while IFS=$'\t' read -r name kind source chart version variants; do
  [[ "$name" =~ ^#.*$ || -z "$name" ]] && continue
  outdir="$DOG/rendered/$name"
  mkdir -p "$outdir"

  ref=""
  case "$kind" in
    repo)
      helm repo add "dogfood-$name" "$source" --force-update >/dev/null
      ref="dogfood-$name/$chart"
      ;;
    oci)
      ref="$source"
      ;;
    local)
      # Chart directory fetched file-by-file from a raw URL base (used for
      # charts that live inside a large repo we don't want to clone).
      src="$DOG/charts-src/$name"
      mkdir -p "$src/templates"
      for f in Chart.yaml values.yaml \
               templates/_helpers.tpl templates/deployment.yaml \
               templates/hpa.yaml templates/ingressroute.yaml \
               templates/rbac.yaml templates/service.yaml \
               templates/serviceaccount.yaml; do
        curl -fsSL "$source/$f" -o "$src/$f"
      done
      # Strip subchart dependencies (traefik, prometheus-adapter for the
      # Triton chart): we lint the chart's own templates, not its bundled
      # third-party infra, and helm refuses to render with them missing.
      sed -i '' '/^dependencies:/,$d' "$src/Chart.yaml"
      ref="$src"
      ;;
  esac

  IFS=',' read -ra vars <<< "$variants"
  for variant in "${vars[@]}"; do
    out="$outdir/$variant.yaml"
    args=(helm template "$name" "$ref")
    [[ "$kind" != "local" ]] && args+=(--version "$version")
    if [[ "$variant" == "gpu" ]]; then
      args+=(--values "hack/dogfood/values/$name-gpu.yaml")
    fi
    log "render $name/$variant ($version)"
    if ! "${args[@]}" > "$out" 2> "$outdir/$variant.render-err.txt"; then
      log "RENDER FAILED: $name/$variant (see $outdir/$variant.render-err.txt)"
      rm -f "$out"
      continue
    fi
    rm -f "$outdir/$variant.render-err.txt"
    # Sanity check the plan insists on: a "gpu" variant must actually
    # contain a GPU resource request, or the render silently tests nothing.
    if [[ "$variant" == "gpu" ]] && ! grep -q 'nvidia.com/' "$out"; then
      log "WARNING: $name/gpu contains no nvidia.com/ resource — bad override?"
    fi
  done
done < hack/dogfood/charts.tsv

# --- Corpus B: example manifests -------------------------------------------
mkdir -p "$DOG/rendered-examples"
while IFS=$'\t' read -r name url; do
  [[ "$name" =~ ^#.*$ || -z "$name" ]] && continue
  out="$DOG/rendered-examples/$name.yaml"
  log "fetch $name"
  if [[ "$url" == *.md ]]; then
    # Extract every ```yaml fenced block from a markdown page, separated by
    # document markers. Fences may be indented (lists, <details> blocks);
    # strip the fence's own indentation from the block body.
    curl -fsSL "$url" | awk '
      match($0, /^[[:space:]]*```(yaml|yml)[[:space:]]*$/) {
        inblock = 1
        indent = index($0, "`") - 1
        print "---"
        next
      }
      inblock && match($0, /^[[:space:]]*```[[:space:]]*$/) { inblock = 0; next }
      inblock { print substr($0, indent + 1) }
    ' > "$out"
  else
    curl -fsSL "$url" -o "$out"
  fi
done < hack/dogfood/examples.tsv

log "done: $(find "$DOG/rendered" "$DOG/rendered-examples" -name '*.yaml' | wc -l | tr -d ' ') rendered files"
