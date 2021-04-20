#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Ted: contact me when you make any changes

go test -race -timeout="90s" -coverprofile="coverage.out" -covermode="atomic" $(go list ./... | grep -v /mocks | grep -v proto)