#!/usr/bin/env bash
# Fails when the public API has incompatible changes since the base commit but the PR title has no
# breaking-change marker, which Release Please would release as a minor or patch version.
#
# Usage: check-api-compatibility.sh <base-commit> <pr-title>
#
# Run from the module root with apidiff on the PATH.
set -euo pipefail

base=$1
title=$2

if [[ $title =~ ^[a-z]+(\([^\)]*\))?!: ]]; then
  echo "The title is marked as a breaking change, so incompatible API changes are allowed."
  exit 0
fi

work=$(mktemp -d)
trap 'git worktree remove --force "$work/base" 2>/dev/null || true; rm -rf "$work"' EXIT

# apidiff loads the module from the current directory, so each side runs from its own checkout
git worktree add --quiet --detach "$work/base" "$base"
(cd "$work/base" && apidiff -m -w "$work/base.export" "$(go list -m)")
apidiff -m -w "$work/head.export" "$(go list -m)"

# apidiff exits 0 either way, so its output is the only signal
incompatible=$(apidiff -m -incompatible "$work/base.export" "$work/head.export")
if [[ -z $incompatible ]]; then
  echo "No incompatible API changes since $base."
  exit 0
fi

cat >&2 <<EOF
This PR makes incompatible changes to the public API:

$incompatible

Without a breaking-change marker, Release Please would release them as a minor or patch version.

- If the break is intended, add ! to the PR title, for example "feat!: ...".
- If it isn't, make the change compatible.
EOF
exit 1
