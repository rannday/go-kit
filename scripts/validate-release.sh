#!/usr/bin/env bash
set -euo pipefail
module=${1:?module required}
version=${2:?version required}
mode=${3:-}
[[ -z $mode || $mode == --inputs-only ]] || exit 2
case "$module" in logx|envx|confx|buildx) ;; *) echo 'Invalid module' >&2; exit 2 ;; esac
# Canonical v0/v1 versions. v2 requires an approved /v2 module migration.
[[ $version =~ ^v(0|1)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]] || { echo 'Invalid canonical v0/v1 version' >&2; exit 2; }
if [[ $version == *-* ]]; then
  IFS=. read -r -a identifiers <<< "${version#*-}"
  for identifier in "${identifiers[@]}"; do
    if [[ $identifier =~ ^[0-9]+$ && $identifier =~ ^0[0-9]+$ ]]; then
      echo 'Leading zero in numeric prerelease identifier' >&2; exit 2
    fi
  done
fi
root=$(cd "$(dirname "$0")/.." && pwd)
expected="github.com/rannday/go-kit/modules/$module"
actual=$(awk '$1 == "module" { print $2 }' "$root/modules/$module/go.mod")
[[ $actual == "$expected" ]] || { echo 'Module declaration mismatch' >&2; exit 1; }
tag="modules/$module/$version"
if [[ $mode != --inputs-only ]] && git -C "$root" show-ref --verify --quiet "refs/tags/$tag"; then echo 'Tag already exists' >&2; exit 1; fi
echo "$tag"
