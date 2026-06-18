#!/bin/bash
set -e

echo "Running Go tests with coverage..."
cd src/go

MODULE=github.com/fitglue/server/src/go

# Include all hand-written code (internal/, pkg/, services/ internals, etc.)
# Only exclude auto-generated protobuf code and E2E test scaffolding.
PACKAGES=$(go list ./... | grep -v \
  -e '/pkg/types/pb' \
  -e '/tests/' \
)

go test -short -coverprofile=coverage.out $PACKAGES > /dev/null

# Strip generated code from the profile before measuring coverage. Generated
# files (oapi-codegen API clients, the enum formatters, protobuf stubs) carry
# the canonical Go "Code generated ... DO NOT EDIT." marker. They are marked
# DO NOT EDIT, so exercising them inflates coverage without testing any
# hand-written FitGlue logic. We still run their tests above (so they keep
# compiling), but exclude them from the coverage metric. Detected dynamically
# so new generated files are excluded automatically.
# Fixed-string match on each generated file's "<import-path>/<file>.go:" prefix.
GEN_PREFIXES=$(grep -rlE '^// Code generated .* DO NOT EDIT\.$' --include='*.go' . \
  | grep -v '_test.go' \
  | sed -E "s#^\./#${MODULE}/#;s#\$#:#")

if [ -n "$GEN_PREFIXES" ]; then
  grep -vFf <(echo "$GEN_PREFIXES") coverage.out > coverage.real.out
else
  cp coverage.out coverage.real.out
fi

# Also exclude package `main.go` entrypoints from the metric. These are thin
# bootstrap/serve wiring (flag parsing, dependency construction, ListenAndServe)
# that is validated by integration/E2E tests, not unit tests. They are exempt
# from the per-file floor below for the same reason, so excluding them here keeps
# the percentage an honest measure of *unit-tested* hand-written logic.
grep -v '/main\.go:[0-9]' coverage.real.out > coverage.real.tmp && mv coverage.real.tmp coverage.real.out

echo "Analyzing coverage (hand-written code only)..."

# Per-file floor: every hand-written file must have SOME coverage. Generated
# files are already excluded above. Package `main.go` entrypoints are exempt —
# they are thin bootstrap/serve wiring, exercised by integration tests rather
# than unit tests.
# Aggregate covered statements per file directly from the profile; a file is a
# violation only if its TOTAL covered statements is zero.
ZERO_FILES=$(awk 'NR>1{f=$1; sub(/:.*/,"",f); s=$(NF-1); c=$NF; tot[f]+=s; if(c>0)cov[f]+=s}
  END{for(k in tot){if(cov[k]==0) print k}}' coverage.real.out \
  | sort -u \
  | grep -v '/main\.go$' || true)

if [ -n "$ZERO_FILES" ]; then
  echo "❌ Error: the following hand-written files have 0% test coverage:"
  echo "$ZERO_FILES" | sed 's#^#   - #'
  echo "Every non-generated file (except main.go entrypoints) must have at least one test."
  exit 1
fi
echo "✅ Every hand-written file (except main.go entrypoints) has some coverage."

COVERAGE=$(go tool cover -func=coverage.real.out | grep total | awk '{print substr($3, 1, length($3)-1)}')
echo "Total Coverage: $COVERAGE%"

# Threshold reflects the current baseline of the codebase.
# Raise this number as coverage improves over time.
THRESHOLD=75
COVERAGE_INT=${COVERAGE%.*}
if [ "$COVERAGE_INT" -lt $THRESHOLD ]; then
  echo "❌ Error: Code coverage ($COVERAGE%) is below the ${THRESHOLD}% requirement."
  echo "Please write more tests before merging."
  exit 1
else
  echo "✅ Code coverage ($COVERAGE%) meets the ${THRESHOLD}% requirement."
fi
