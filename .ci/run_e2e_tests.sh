SCRIPTS_PATH=$(cd $(dirname "${BASH_SOURCE[0]}"); pwd)
SRC_PATH=$(dirname "${SCRIPTS_PATH}")

# Early auth to avoid limit rating
if [[ -z ${DOCKER_USERNAME} ]]; then
    echo "Skipping E2E Tests for untrusted build"
    exit 0
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
BYZANTINE_IMAGE="$DOCKER_REPO/avalanche-byzantine:v0.2.0-rc.1"
TEST_SUITE_IMAGE="$DOCKER_REPO/avalanche-testing:v0.11.0-rc.2"

# Kurtosis Environment Parameters
KURTOSIS_CORE_CHANNEL="1.0.3"
INITIALIZER_IMAGE="kurtosistech/kurtosis-core_initializer:${KURTOSIS_CORE_CHANNEL}"
API_IMAGE="kurtosistech/kurtosis-core_api:${KURTOSIS_CORE_CHANNEL}"
PARALLELISM=4

docker pull "${BYZANTINE_IMAGE}"
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
