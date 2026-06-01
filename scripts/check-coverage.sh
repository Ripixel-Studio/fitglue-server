#!/bin/bash
set -e

echo "Running Go tests with coverage..."
cd src/go

# Include all hand-written code (internal/, pkg/, services/ internals, etc.)
# Only exclude auto-generated protobuf code and E2E test scaffolding.
PACKAGES=$(go list ./... | grep -v \
  -e '/pkg/types/pb' \
  -e '/tests/' \
)

go test -short -coverprofile=coverage.out $PACKAGES > /dev/null

echo "Analyzing coverage..."
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print substr($3, 1, length($3)-1)}')
echo "Total Coverage: $COVERAGE%"

# Threshold reflects the current baseline of the codebase.
# Raise this number as coverage improves over time.
THRESHOLD=25
COVERAGE_INT=${COVERAGE%.*}
if [ "$COVERAGE_INT" -lt $THRESHOLD ]; then
  echo "❌ Error: Code coverage ($COVERAGE%) is below the ${THRESHOLD}% requirement."
  echo "Please write more tests before merging."
  exit 1
else
  echo "✅ Code coverage ($COVERAGE%) meets the ${THRESHOLD}% requirement."
fi
