#!/usr/bin/env bash
# Re-runs a CI run once when GitHub never gave one of its jobs a runner.
#
# GitHub's hosted macOS arm64 pool sometimes cannot place a job. It sits queued
# for about 15 minutes, runs no step, and fails with "The job was not acquired by
# Runner of type hosted even after multiple attempts". On 2026-11-02 that failed
# five macOS legs across three pull requests, while runs queued minutes either
# side got a runner in seconds, and each one needed a re-run by hand. Nothing in
# the code failed, so nothing in the code can fix it; the job only needs asking
# again.
#
# This asks again, once, and only when that is all that went wrong:
#   - attempt 1 only, so a pool that stays unavailable costs one retry, not a loop;
#   - only when every failed job carries GitHub's not-acquired annotation, so a
#     real failure is never retried away (and a flaky test never hidden);
#   - only when the run is still the newest for its branch, so a re-run never
#     cancels the run that superseded it (CI cancels in-progress PR runs).
#
# Usage: requeue-stranded.sh <run-id> <attempt> [--dry-run]
# Needs GH_TOKEN (actions: write, checks: read) and GITHUB_REPOSITORY (owner/name).
set -euo pipefail

usage='usage: requeue-stranded.sh <run-id> <attempt> [--dry-run]'
run=${1:?$usage}
attempt=${2:?$usage}
dry=${3:-}
repo=${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY to owner/name}
marker='was not acquired by Runner'

if [ "$attempt" != 1 ]; then
  echo "run $run is on attempt $attempt; only the first attempt is re-run automatically"
  exit 0
fi

failed=$(gh api "repos/$repo/actions/runs/$run/attempts/$attempt/jobs?per_page=100" \
  --jq '.jobs[] | select(.conclusion == "failure" or .conclusion == "cancelled" or .conclusion == "timed_out") | "\(.id)\t\(.name)"')
if [ -z "$failed" ]; then
  echo "run $run has no failed job"
  exit 0
fi

stranded=0
other=0
while IFS=$'\t' read -r id name; do
  # Captured, not piped into `grep -q`: grep exits at the first match, gh takes
  # SIGPIPE, and pipefail would turn a match into a miss.
  messages=$(gh api "repos/$repo/check-runs/$id/annotations" --jq '.[].message')
  case $messages in
    *"$marker"*) echo "never got a runner: $name"; stranded=$((stranded + 1)) ;;
    *) echo "failed on its own: $name"; other=$((other + 1)) ;;
  esac
done <<<"$failed"

if [ "$other" -gt 0 ]; then
  echo "$other job(s) failed on their own; leaving run $run for a person to look at"
  exit 0
fi

read -r workflow branch < <(gh api "repos/$repo/actions/runs/$run" --jq '"\(.workflow_id) \(.head_branch)"')
newest=$(gh api "repos/$repo/actions/workflows/$workflow/runs?branch=$branch&per_page=1" --jq '.workflow_runs[0].id')
if [ "$newest" != "$run" ]; then
  echo "run $newest is newer for $branch; leaving run $run alone"
  exit 0
fi

if [ "$dry" = --dry-run ]; then
  echo "would re-run the $stranded stranded job(s) of run $run"
  exit 0
fi
gh api -X POST "repos/$repo/actions/runs/$run/rerun-failed-jobs" >/dev/null
echo "re-running the $stranded stranded job(s) of run $run"
