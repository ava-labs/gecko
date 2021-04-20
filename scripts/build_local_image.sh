#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Build image from current local source
SRC_DIR="$(dirname "${BASH_SOURCE[0]}")"
AVALANCHE_PATH=$( cd "$( dirname "${BASH_SOURCE[0]}" )"; cd .. && pwd ) # Directory above this script
DOCKERHUB_REPO="avaplatform/avalanchego"

# WARNING: this will use the most recent commit even if there are un-committed changes present
FULL_COMMIT_HASH="$(git --git-dir="$AVALANCHE_PATH/.git" rev-parse HEAD)"
COMMIT_HASH="${FULL_COMMIT_HASH::8}"

echo "Building Docker Image: $DOCKERHUB_REPO:$COMMIT_HASH"
docker build -t "$DOCKERHUB_REPO:$COMMIT_HASH" "$AVALANCHE_PATH" -f "$AVALANCHE_PATH/Dockerfile" --build-arg AVALANCHEGO_COMMIT="$FULL_COMMIT_HASH"
