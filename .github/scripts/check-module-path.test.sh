#!/usr/bin/env bash
set -euo pipefail

script="$(dirname "$0")/check-module-path.sh"
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
failures=0

# check <pass|fail> <mode> <manifest version, or - for none> <module path suffix> [expected output]
check() {
  local want=$1 mode=$2 version=$3 suffix=$4 expected=${5:-}
  rm -f "$dir/manifest.json"
  [[ $version == - ]] || echo "{\".\": \"$version\"}" >"$dir/manifest.json"
  echo "module github.com/OctopusDeploy/openfeature-provider-go$suffix" >"$dir/go.mod"

  local args=() got=pass output
  [[ $mode == release ]] && args=(--release)
  output=$("$script" "${args[@]}" "$dir/manifest.json" "$dir/go.mod" 2>&1) || got=fail

  local name="$mode $version ${suffix:-(no suffix)}"
  if [[ $got != "$want" ]]; then
    echo "FAIL $name: wanted $want, got $got: $output"
    failures=$((failures + 1))
  elif [[ -n $expected && $output != *"$expected"* ]]; then
    echo "FAIL $name: output doesn't mention $expected: $output"
    failures=$((failures + 1))
  else
    echo "ok   $name"
  fi
}

check pass release - ""
check pass release 0.3.0 ""
check pass release 1.2.0 ""
check pass release 2.0.0 /v2
check fail release 2.0.0 "" BEGIN_COMMIT_OVERRIDE
check fail release 1.4.0 /v2 "Release-As: 2.0.0"
check fail release 1.0.0 /v1

check pass lenient 1.4.0 /v2
check fail lenient 0.3.0 /v2
check fail lenient 1.4.0 /v3

((failures == 0))
