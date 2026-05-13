#!/usr/bin/env bash
# Seeds GitHub Issues for the FaaS MVP backlog.
#
# Reads .github/issues-manifest.tsv and .github/issue-bodies/*.md
# and creates labels, milestones, and issues in the current repo.
#
# Requirements:
#   - gh CLI authenticated (gh auth login)
#   - jq
#
# Idempotency:
#   - Labels are created/updated with --force.
#   - Milestones: skipped if a milestone with the same title already exists.
#   - Issues: skipped if any issue (open or closed) already starts with the same T-id.
#
# Usage:
#   scripts/seed-github-issues.sh             create everything
#   scripts/seed-github-issues.sh --dry-run   print actions without changing anything

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="$REPO_ROOT/.github/issues-manifest.tsv"
BODIES_DIR="$REPO_ROOT/.github/issue-bodies"

DRY_RUN=0
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
  echo "DRY-RUN mode: no changes will be made"
fi

run() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf 'DRY-RUN: '
    printf '%q ' "$@"
    printf '\n'
  else
    "$@"
  fi
}

require() {
  command -v "$1" >/dev/null 2>&1 || { echo "Missing required tool: $1" >&2; exit 1; }
}

require gh
require jq

gh auth status >/dev/null 2>&1 || {
  echo "gh is not authenticated. Run 'gh auth login' first." >&2
  exit 1
}

REPO="$(gh repo view --json nameWithOwner -q .nameWithOwner)"
echo "Target repo: $REPO"

############################################
# Labels
############################################
echo "==> Labels"

# Format: name|color|description
LABELS=(
  "milestone:M0|0E8A16|Milestone M0 — Foundation"
  "milestone:M1|0E8A16|Milestone M1 — Infra"
  "milestone:M2|0E8A16|Milestone M2 — Lifecycle"
  "milestone:M3|0E8A16|Milestone M3 — E2E"
  "milestone:M4|0E8A16|Milestone M4 — Load"
  "size:S|C5DEF5|Size: small"
  "size:M|BFD4F2|Size: medium"
  "size:L|5319E7|Size: large"
  "area:proto|FBCA04|Area: proto / API contract"
  "area:pkg|FBCA04|Area: shared pkg/"
  "area:control-plane|FBCA04|Area: control-plane"
  "area:gateway|FBCA04|Area: api-gateway"
  "area:reconciler|FBCA04|Area: K8s reconciler"
  "area:storage|FBCA04|Area: storage / Postgres"
  "area:observability|FBCA04|Area: metrics / tracing / logs"
  "area:helm|FBCA04|Area: Helm chart"
  "area:terraform|FBCA04|Area: Terraform"
  "area:e2e|FBCA04|Area: integration tests"
  "area:load|FBCA04|Area: load tests"
  "area:ci|FBCA04|Area: CI"
)

for spec in "${LABELS[@]}"; do
  IFS='|' read -r name color desc <<<"$spec"
  run gh label create "$name" --color "$color" --description "$desc" --force
done

############################################
# Milestones
############################################
echo "==> Milestones"

MILESTONES=(
  "M0 Foundation"
  "M1 Infra"
  "M2 Lifecycle"
  "M3 E2E"
  "M4 Load"
)

existing_ms="$(gh api "repos/$REPO/milestones?state=all&per_page=100" --jq '.[].title')"

for ms in "${MILESTONES[@]}"; do
  if grep -Fxq "$ms" <<<"$existing_ms"; then
    echo "milestone: \"$ms\" — exists, skipping"
  else
    run gh api "repos/$REPO/milestones" -f title="$ms" -f state="open" >/dev/null
    echo "milestone: \"$ms\" — created"
  fi
done

############################################
# Issues
############################################
echo "==> Issues"

[[ -f "$MANIFEST" ]] || { echo "Manifest not found: $MANIFEST" >&2; exit 1; }
[[ -d "$BODIES_DIR" ]] || { echo "Bodies dir not found: $BODIES_DIR" >&2; exit 1; }

# Pre-fetch existing issue titles once to avoid hammering the API.
existing_titles="$(gh issue list --state all --limit 500 --json title --jq '.[].title' || true)"

create_count=0
skip_count=0
warn_count=0

while IFS=$'\t' read -r id title milestone size areas; do
  [[ "$id" == "id" || -z "$id" ]] && continue

  body_file="$BODIES_DIR/$id.md"
  if [[ ! -f "$body_file" ]]; then
    echo "WARN: missing body for $id, skipping" >&2
    warn_count=$((warn_count + 1))
    continue
  fi

  full_title="$id $title"

  if grep -Fq "$id " <<<"$existing_titles"; then
    echo "$full_title — exists, skipping"
    skip_count=$((skip_count + 1))
    continue
  fi

  ms_num="${id#T}"
  ms_num="${ms_num%%.*}"
  labels="milestone:M${ms_num},size:${size}"
  IFS=',' read -ra area_arr <<<"$areas"
  for a in "${area_arr[@]}"; do
    labels="${labels},area:${a}"
  done

  echo "creating: $full_title  [labels: $labels]"
  run gh issue create \
    --title "$full_title" \
    --milestone "$milestone" \
    --label "$labels" \
    --body-file "$body_file" >/dev/null
  create_count=$((create_count + 1))
done < "$MANIFEST"

echo "==> Done. created=$create_count, skipped=$skip_count, warned=$warn_count"
