#!/bin/bash
set -e

# Usage: ./scripts/upload_sourcemaps.sh <RELEASE_VERSION>
RELEASE=$1

if [ -z "$RELEASE" ]; then
  echo "Error: Release version argument is required"
  exit 1
fi

if [ -z "$SENTRY_AUTH_TOKEN" ]; then
  echo "Error: SENTRY_AUTH_TOKEN environment variable is required"
  exit 1
fi

ORG="ripixel-studio"
PROJECT="server"
TS_ROOT="src/typescript"

# Create release if it doesn't exist
echo "Creating release $RELEASE..."
npx @sentry/cli releases new "$RELEASE" --org "$ORG" --project "$PROJECT" || true

# 1. Upload Shared Library Source Maps
echo "Uploading source maps for shared library..."
SHARED_DIST="$TS_ROOT/shared/dist"
if [ -d "$SHARED_DIST" ]; then
  npx @sentry/cli sourcemaps upload "$SHARED_DIST" \
    --release "$RELEASE" \
    --url-prefix "~/shared/dist" \
    --org "$ORG" \
    --project "$PROJECT" \
    --ext js --ext map \
    --ignore "*.test.js" --ignore "*.test.js.map"
else
  echo "Warning: No dist directory found for shared library. Skipping."
fi

# 2. Iterate over all TypeScript workspaces (excluding shared, admin-cli, mcp-server, node_modules)
for dir in $TS_ROOT/*; do
  if [ -d "$dir" ] && [ -f "$dir/package.json" ]; then
    NAME=$(basename "$dir")

    # Skip excluded directories
    if [[ "$NAME" == "shared" || "$NAME" == "admin-cli" || "$NAME" == "mcp-server" || "$NAME" == "node_modules" ]]; then
      continue
    fi

    DIST_DIR="$dir/dist"

    if [ ! -d "$DIST_DIR" ]; then
      echo "Warning: No dist directory found for $NAME. Skipping."
      continue
    fi

    echo "Uploading source maps for $NAME..."

    # URL Prefix strategy:
    # Runtime path: /workspace/{name}/dist/index.js
    # Sentry Artifact: ~/{name}/dist/index.js
    URL_PREFIX="~/$NAME/dist"

    # Upload everything in dist dir (js and maps), excluding tests
    npx @sentry/cli sourcemaps upload "$DIST_DIR" \
      --release "$RELEASE" \
      --url-prefix "$URL_PREFIX" \
      --org "$ORG" \
      --project "$PROJECT" \
      --ext js --ext map \
      --ignore "*.test.js" --ignore "*.test.js.map"
  fi
done

echo "Finalizing release $RELEASE..."
npx @sentry/cli releases finalize "$RELEASE" --org "$ORG" --project "$PROJECT"
echo "Source map upload complete!"
