#!/usr/bin/env bash
set -euo pipefail

BIN="${NORMALCTL:-}"
DIR="testdata/invariants"
MANIFEST="$DIR/manifest.json"

if ! command -v jq >/dev/null 2>&1; then
  echo "check-invariants: jq is required" >&2
  exit 2
fi

if [ -z "$BIN" ]; then
  BIN="$(mktemp -d)/normalctl"
  go build -o "$BIN" ./cmd/normalctl
fi

NOW="$(jq -r '.now' "$MANIFEST")"
failures=0
checked=0

echo "verifying invariants against $BIN at now=$NOW"
echo

while IFS=$'\t' read -r file why; do
  checked=$((checked + 1))
  if "$BIN" validate --now "$NOW" "$DIR/$file" >/dev/null 2>&1; then
    echo "FAIL  $file was accepted"
    echo "      invariant: $why"
    failures=$((failures + 1))
  else
    echo "ok    rejected $file"
  fi
done < <(jq -r '.reject[] | [.file, .why] | @tsv' "$MANIFEST")

while IFS=$'\t' read -r file why; do
  checked=$((checked + 1))
  if output="$("$BIN" validate --now "$NOW" "$DIR/$file" 2>&1)"; then
    echo "ok    accepted $file"
  else
    echo "FAIL  $file was rejected"
    echo "      expectation: $why"
    echo "$output" | sed 's/^/      /'
    failures=$((failures + 1))
  fi
done < <(jq -r '.accept[] | [.file, .why] | @tsv' "$MANIFEST")

echo
if [ "$failures" -ne 0 ]; then
  echo "$failures of $checked invariant fixtures behaved unexpectedly"
  exit 1
fi

if [ "$checked" -lt 20 ]; then
  echo "only $checked fixtures ran; the corpus looks truncated"
  exit 1
fi

echo "all $checked invariant fixtures behaved as declared"
