#!/usr/bin/env bash

set -ev

bash <(curl -s https://codecov.io/bash)

# Skip if this is not on the main public repo or
# if this is not a trusted build (Docker Credentials are not set)
if [[ $TRAVIS_REPO_SLUG != "ava-labs/avalanchego" || -z "$DOCKER_USERNAME"  ]]; then
  exit 0;
fi

FULL_COMMIT_HASH="$(git --git-dir="$AVALANCHE_HOME/.git" rev-parse HEAD)"
COMMIT="${FULL_COMMIT_HASH::8}"

AVALANCHE_IMAGE="$DOCKERHUB_REPO:$COMMIT"

TRAVIS_IMAGE_TAG="$DOCKERHUB_REPO:travis-$TRAVIS_BUILD_NUMBER"
docker tag "$AVALANCHE_IMAGE" "$TRAVIS_IMAGE_TAG"

if [[ $TRAVIS_BRANCH == "master" ]]; then
  echo "Tagging $AVALANCHE_IMAGE as $DOCKERHUB_REPO:latest"
  docker tag "$AVALANCHE_IMAGE" "$DOCKERHUB_REPO:latest"
fi

if [[ $TRAVIS_BRANCH == "dev" ]]; then
  echo "Tagging $AVALANCHE_IMAGE as $DOCKERHUB_REPO:dev"
  docker tag "$AVALANCHE_IMAGE" "$DOCKERHUB_REPO:dev"
fi

if [[ $TRAVIS_TAG != "" ]]; then
  echo "Tagging $AVALANCHE_IMAGE as $DOCKERHUB_REPO:$TRAVIS_TAG"
  docker tag "$AVALANCHE_IMAGE" "$DOCKERHUB_REPO:$TRAVIS_TAG"
fi

echo "$DOCKER_PASS" | docker login --username "$DOCKER_USERNAME" --password-stdin

# following should push all tags
docker push $DOCKERHUB_REPO
