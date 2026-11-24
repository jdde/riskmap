#!/bin/sh
# Measures the test coverage of the CLI and fails if it is below the minimum.
#
#     sh scripts/coverage.sh          run from the repository root
#     MIN=99.5 sh scripts/coverage.sh try a different minimum
#
# Coverage is the share of statements in cmd/ and internal/ that at least one
# test executes. All tests count together, so a line in internal/eval that is
# only reached by the acceptance tests under tests/ is covered.
#
# In GitHub Actions the report is also written to the job summary.
set -eu

MIN="${MIN:-99}"
profile="${TMPDIR:-/tmp}/riskmap-coverage.$$"
trap 'rm -f "$profile"' EXIT

go test -count=1 -covermode=atomic -coverpkg=./cmd/...,./internal/... -coverprofile="$profile" ./... > /dev/null

report=$(go tool cover -func="$profile")
total=$(printf '%s\n' "$report" | awk '$1 == "total:" { sub("%", "", $3); print $3 }')
# Functions that are not fully covered, least covered first.
gaps=$(printf '%s\n' "$report" | awk '$1 != "total:" && $3 != "100.0%" { sub("github.com/jdde/riskmap/", "", $1); printf "%7s  %s %s\n", $3, $1, $2 }' | sort -n)

echo "coverage: ${total}% of statements (minimum ${MIN}%)"
if [ -n "$gaps" ]; then
    echo
    echo "not fully covered:"
    echo "$gaps"
fi

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    {
        echo "### Test coverage: ${total}% (minimum ${MIN}%)"
        echo
        echo '```'
        echo "${gaps:-every function is fully covered}"
        echo '```'
    } >> "$GITHUB_STEP_SUMMARY"
fi

# The shell cannot compare decimal numbers, awk can.
if awk -v total="$total" -v min="$MIN" 'BEGIN { exit !(total < min) }'; then
    echo
    echo "FAILED: coverage ${total}% is below the minimum of ${MIN}%" >&2
    exit 1
fi
