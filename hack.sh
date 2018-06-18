# vendor/k8s.io/code-generator/generate-groups.sh deepcopy \
# github.com/ovrclk/akash/pkg/client \ github.com/ovrclk/akash/pkg/apis \
# akash.io:v1o


#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${SCRIPT_ROOT}; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ${GOPATH}/src/k8s.io/code-generator)}

vendor/k8s.io/code-generator/generate-groups.sh deepcopy \
  github.com/ovrclk/akash/pkg/client github.com/ovrclk/akash/pkg/apis \
  akash.io:v1 \


vendor/k8s.io/code-generator/generate-groups.sh deepcopy \
  github.com/ovrclk/akash/types github.com/ovrclk/akash/pkg/apis \
  akash.io:v1 \
