#!/usr/bin/env bash
# Fails when the module path's /vN suffix doesn't match the release version.
#
# Usage: check-module-path.sh [--release] <manifest> <go.mod>
#
# With --release, the manifest holds the version Release Please is about to tag, so the suffix must
# match it exactly. Otherwise it holds the last release, and the suffix may also be one major ahead,
# so a v2 path change can land on main before the release PR that tags it.
set -euo pipefail

release=false
if [[ ${1:-} == --release ]]; then
  release=true
  shift
fi
manifest=$1
gomod=$2

if [[ ! -f $manifest ]]; then
  echo "No $manifest, so there's no release version to check."
  exit 0
fi

version=$(jq -r '."." // empty' "$manifest")
if [[ ! $version =~ ^([0-9]+)\. ]]; then
  echo "$manifest has no version under \".\"." >&2
  exit 1
fi
major=${BASH_REMATCH[1]}

module=$(awk '$1 == "module" { print $2 }' "$gomod")
path_major=1
if [[ $module =~ /v([0-9]+)$ ]]; then
  path_major=${BASH_REMATCH[1]}
  if ((path_major < 2)); then
    echo "go.mod declares $module, but /v0 and /v1 aren't valid suffixes. Remove it." >&2
    exit 1
  fi
fi

# Majors 0 and 1 share the unsuffixed path
effective() { echo $(($1 < 2 ? 1 : $1)); }
suffix() { (($1 < 2)) && echo "no suffix" || echo "/v$1"; }

proposed=$(effective "$major")

if $release; then
  if ((path_major == proposed)); then
    echo "Release $version matches module path $module."
    exit 0
  fi

  if ((proposed > path_major)); then
    cat >&2 <<EOF
The release PR proposes $version, but go.mod declares $module. From v2 onwards the module path must end in /v$major.

- If this is meant to be v$major, merge a PR to main that changes the module line in go.mod and the module's own imports to /v$major.
- If it isn't, remove the breaking-change marker. Add a BEGIN_COMMIT_OVERRIDE section with the corrected commit message to the description of the merged PR that introduced it, then re-run the release workflow.
EOF
  else
    cat >&2 <<EOF
The release PR proposes $version, but go.mod declares $module, which is the path for v$path_major.

- If this is meant to be v$path_major, merge a commit to main with Release-As: $path_major.0.0 in its description.
- If it isn't, revert the /v$path_major module path change.
EOF
  fi
  exit 1
fi

next=$(effective $((major + 1)))
if ((path_major == proposed || path_major == next)); then
  echo "Module path $module is valid after release $version."
  exit 0
fi

allowed=$(suffix "$proposed")
if ((next != proposed)); then
  allowed="$allowed or $(suffix "$next")"
fi
echo "go.mod declares $module, but the last release is $version, so the module path should have $allowed." >&2
exit 1
