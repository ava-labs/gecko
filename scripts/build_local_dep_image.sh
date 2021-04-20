#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

echo "Building docker image based off of most recent local commits of avalanchego and coreth"

AVALANCHE_REMOTE="git@github.com:ava-labs/avalanchego.git"
CORETH_REMOTE="git@github.com:ava-labs/coreth.git"
DOCKERHUB_REPO="avaplatform/avalanchego"

DOCKER="${DOCKER:-docker}"
SCRIPT_DIRPATH=$(cd $(dirname "${BASH_SOURCE[0]}") && pwd)
ROOT_DIRPATH="$(dirname "${SCRIPT_DIRPATH}")"

AVA_LABS_RELATIVE_PATH="src/github.com/ava-labs"
EXISTING_GOPATH="$GOPATH"

export GOPATH="$SCRIPT_DIRPATH/.build_image_gopath"
WORKPREFIX="$GOPATH/src/github.com/ava-labs"

# Clone the remotes and checkout the desired branch/commits
AVALANCHE_CLONE="$WORKPREFIX/avalanchego"
CORETH_CLONE="$WORKPREFIX/coreth"

# Replace the WORKPREFIX directory
rm -rf "$WORKPREFIX"
mkdir -p "$WORKPREFIX"


AVALANCHE_COMMIT_HASH="$(git -C "$EXISTING_GOPATH/$AVA_LABS_RELATIVE_PATH/avalanchego" rev-parse --short HEAD)"
CORETH_COMMIT_HASH="$(git -C "$EXISTING_GOPATH/$AVA_LABS_RELATIVE_PATH/coreth" rev-parse --short HEAD)"

git config --global credential.helper cache

git clone "$AVALANCHE_REMOTE" "$AVALANCHE_CLONE"
git -C "$AVALANCHE_CLONE" checkout "$AVALANCHE_COMMIT_HASH"

git clone "$CORETH_REMOTE" "$CORETH_CLONE"
git -C "$CORETH_CLONE" checkout "$CORETH_COMMIT_HASH"

CONCATENATED_HASHES="$AVALANCHE_COMMIT_HASH-$CORETH_COMMIT_HASH"

"$DOCKER" build -t "$DOCKERHUB_REPO:$CONCATENATED_HASHES" "$WORKPREFIX" -f "$SCRIPT_DIRPATH/local.Dockerfile"
