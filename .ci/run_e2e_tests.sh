SCRIPTS_PATH=$(cd $(dirname "${BASH_SOURCE[0]}"); pwd)
SRC_PATH=$(dirname "${SCRIPTS_PATH}")

# Early auth to avoid limit rating
if [[ -z ${DOCKER_USERNAME} ]]; then
    echo "No Auth provided"
else
    echo "$DOCKER_PASS" | docker login --username "$DOCKER_USERNAME" --password-stdin
fi

# Build the runnable Avalanche docker image
bash "${SRC_PATH}"/scripts/build_image.sh

AVALANCHE_IMAGE_REPO=$(docker image ls --format="{{.Repository}}" | head -n 1)
AVALANCHE_IMAGE_TAG=$(docker image ls --format="{{.Tag}}" | head -n 1)
AVALANCHE_IMAGE="$AVALANCHE_IMAGE_REPO:$AVALANCHE_IMAGE_TAG"

echo "Using Avalanche Image: $AVALANCHE_IMAGE"

DOCKER_REPO="avaplatform"
AVALANCHE_TESTING_REPO=$DOCKER_REPO/avalanche-testing
DEFAULT_TEST_SUITE_IMAGE="$AVALANCHE_TESTING_REPO:dev"
BYZANTINE_IMAGE="$DOCKER_REPO/avalanche-byzantine:v0.1.5-rc.1"
CI_COMMIT_TAG="avalanchego-ci-${COMMIT}"


function docker_tag_exists() {
    TOKEN=$(curl -s -H "Content-Type: application/json" -X POST -d '{"username": "'${DOCKER_USERNAME}'", "password": "'${DOCKER_PASS}'"}' https://hub.docker.com/v2/users/login/ | jq -r .token)
    curl --silent -f --head -lL https://hub.docker.com/v2/repositories/$1/tags/$2/ > /dev/null
}

if docker_tag_exists $AVALANCHE_TESTING_REPO $CI_COMMIT_TAG; then
    echo "$AVALANCHE_TESTING_REPO:$CI_COMMIT_TAG exists; using this image to run e2e tests" 
    TEST_SUITE_IMAGE="$AVALANCHE_TESTING_REPO:$CI_COMMIT_TAG"
elif docker_tag_exists $AVALANCHE_TESTING_REPO $BRANCH; then
    echo "$AVALANCHE_TESTING_REPO:$BRANCH exists; using this image to run e2e tests" 
    TEST_SUITE_IMAGE="$AVALANCHE_TESTING_REPO:$BRANCH"
else
    echo "$AVALANCHE_TESTING_REPO $BRANCH does NOT exist; using the default image to run e2e tests" 
    TEST_SUITE_IMAGE=$DEFAULT_TEST_SUITE_IMAGE
fi

echo "Using $TEST_SUITE_IMAGE for e2e tests"

# If Docker Credentials are not available skip the Byzantine Tests
if [[ -z ${DOCKER_USERNAME} ]]; then
    echo "Skipping Byzantine Tests because Docker Credentials were not present."
    BYZANTINE_IMAGE=""
else
    docker pull "${BYZANTINE_IMAGE}"
fi

# Kurtosis Environment Parameters
KURTOSIS_CORE_CHANNEL="1.0.3"
INITIALIZER_IMAGE="kurtosistech/kurtosis-core_initializer:${KURTOSIS_CORE_CHANNEL}"
API_IMAGE="kurtosistech/kurtosis-core_api:${KURTOSIS_CORE_CHANNEL}"
PARALLELISM=4

docker pull "$TEST_SUITE_IMAGE"

SUITE_EXECUTION_VOLUME="avalanche-test-suite_${AVALANCHE_IMAGE_TAG}_$(date +%s)"
docker volume create "${SUITE_EXECUTION_VOLUME}"

# Docker only allows you to have spaces in the variable if you escape them or use a Docker env file
CUSTOM_ENV_VARS_JSON="CUSTOM_ENV_VARS_JSON={\"AVALANCHE_IMAGE\":\"${AVALANCHE_IMAGE}\",\"BYZANTINE_IMAGE\":\"${BYZANTINE_IMAGE}\"}"

echo "${CUSTOM_ENV_VARS_JSON}"
echo "${KURTOSIS_API_IMAGE}"
echo "${INITIALIZER_IMAGE}"
docker run \
    --mount "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock" \
    --mount "type=volume,source=${SUITE_EXECUTION_VOLUME},target=/suite-execution" \
    --env "${CUSTOM_ENV_VARS_JSON}" \
    --env "TEST_SUITE_IMAGE=${TEST_SUITE_IMAGE}" \
    --env "SUITE_EXECUTION_VOLUME=${SUITE_EXECUTION_VOLUME}" \
    --env "KURTOSIS_API_IMAGE=${API_IMAGE}" \
    --env "PARALLELISM=${PARALLELISM}" \
    `# In Bash, this is how you feed arguments exactly as-is to a child script (since ${*} loses quoting and ${@} trips set -e if no arguments are passed)` \
    `# It basically says, "if and only if ${1} exists, evaluate ${@}"` \
    ${1+"${@}"} \
    "${INITIALIZER_IMAGE}"

# If the E2E Tests Passed, tag and push the E2E Test Suite Image
if [[ $? ==  0 ]]; then
    TEST_SUITE_IMAGE_TAG="${AVALANCHE_TESTING_REPO}:${CI_COMMIT_TAG}"
    echo "Tagging E2E Tests from Successful run as ${TEST_SUITE_IMAGE_TAG}"
    docker tag "${TEST_SUITE_IMAGE}" "${TEST_SUITE_IMAGE_TAG}"
    docker push "${TEST_SUITE_IMAGE_TAG}"
fi

