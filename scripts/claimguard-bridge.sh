#!/usr/bin/env bash
# Dogfood: if claimguard is on PATH, re-check a fixture (or JOBKIT_CLAIMGUARD_FILE)
# using jobkit claims.yaml when present.
set -euo pipefail
if ! command -v claimguard >/dev/null 2>&1; then
  echo "claimguard-bridge: claimguard not installed; skip (install claimguard to enable)"
  exit 0
fi
HOME_JOBKIT="${JOBKIT_HOME:-$HOME/.jobkit}"
CLAIMS="${HOME_JOBKIT}/claims.yaml"
if [[ ! -f "$CLAIMS" ]]; then
  # use demo claims from claimguard if packaging relative
  echo "claimguard-bridge: no $CLAIMS; skip"
  exit 0
fi
FILE="${JOBKIT_CLAIMGUARD_FILE:-}"
if [[ -z "$FILE" ]]; then
  # synthesize a tiny good file from allowlist first entry for smoke — prefer external file
  echo "claimguard-bridge: set JOBKIT_CLAIMGUARD_FILE to a draft to check; running self-smoke on empty allow-only"
  tmp="$(mktemp)"
  # empty body is ok
  printf 'No quantified claims here.\n' >"$tmp"
  claimguard check --claims "$CLAIMS" --file "$tmp"
  rm -f "$tmp"
  echo "claimguard-bridge: OK"
  exit 0
fi
claimguard check --claims "$CLAIMS" --file "$FILE"
echo "claimguard-bridge: OK"
