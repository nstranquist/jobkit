#!/usr/bin/env bash
# End-to-end jobkit walkthrough against an isolated JOBKIT_HOME.
set -euo pipefail
cd "$(dirname "$0")/.."

: "${JOBKIT_HOME:?set JOBKIT_HOME to an isolated dir}"
BIN=./bin/jobkit
JD=examples/jd-backend.txt
export JOBKIT_PROFILE=examples/profile.yaml

step() { printf '\n\033[1m== %s ==\033[0m\n' "$1"; }

step "profile validate"
$BIN profile validate

step "eligibility policy + explainable check"
$BIN eligibility init --home "Austin, TX" --countries "United States,USA,US" --languages English --years 7 --relocation-open
$BIN eligibility check $JD --location "Remote - US" --remote

step "jd parse"
$BIN jd parse $JD | head -12

step "match (gap report)"
$BIN match $JD | head -30

step "resume build (tailored markdown)"
$BIN resume build $JD | head -20

step "resume build (html artifact)"
$BIN resume build $JD --format html --out "$JOBKIT_HOME/resume.html"
ls -la "$JOBKIT_HOME/resume.html"

step "letter build"
$BIN letter build $JD --tone direct | head -20

step "prep (interview sheet)"
$BIN prep $JD | head -16

step "apply (golden path)"
$BIN apply $JD --tone direct --location "Remote - US" --remote

step "inbox + weekly slate"
$BIN inbox add $JD --location "Remote - US" --remote --source demo
$BIN inbox slate

step "track lifecycle"
$BIN track add Globex "Platform Engineer" --url https://globex.example/jobs/7 --note "via referral"
$BIN track set globex --status applied
$BIN track set initech --status applied
$BIN track list
$BIN track stats

step "json envelope"
$BIN match $JD --json | head -8

echo
echo "demo complete (state in $JOBKIT_HOME)"
