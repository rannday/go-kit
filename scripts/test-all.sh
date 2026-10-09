#!/usr/bin/env bash
set -uo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
mode=isolated
module=
race=false
while (($#)); do
  case "$1" in
    --workspace) mode=workspace ;;
    --module) shift; module=${1:?missing module} ;;
    --race) race=true ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done
if [[ -n $module && ! $module =~ ^(logx|envx|confx|buildx)$ ]]; then exit 2; fi
# Child Go commands in buildx's temporary test modules must discover their own scope.
if [[ $mode == isolated ]]; then export GOWORK=off; else export GOWORK=auto; fi
go version || exit 1
failed=0
check() { echo "CHECK $name: $*"; "$@" || failed=1; }
state() {
  for f in go.mod go.sum; do
    if [[ -f $f ]]; then sha256sum "$f"; else echo "$f absent"; fi
  done
}
selected=0
for file in "$root"/modules/*/go.mod; do
  [[ -f $file ]] || continue
  dir=${file%/go.mod}; name=${dir##*/}
  [[ -z $module || $name == "$module" ]] || continue
  selected=$((selected + 1))
  pushd "$dir" >/dev/null || exit 1
  unformatted=$(gofmt -l .) || failed=1
  if [[ -n $unformatted ]]; then echo "$unformatted"; failed=1; fi
  if [[ $mode == isolated ]]; then
    before=$(state)
    check go mod tidy
    [[ $(state) == "$before" ]] || { echo "$name: tidy changed module files"; failed=1; }
  fi
  check go test -mod=readonly ./...
  if $race; then check go test -mod=readonly -race ./...; fi
  check go build ./...
  check go vet ./...
  popd >/dev/null || exit 1
done
((selected)) || exit 1
exit "$failed"
