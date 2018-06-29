#!/bin/bash

set -euo pipefail

do_glide(){
  echo "--- :building_construction: installing glide"
  curl https://glide.sh/get | sh

  echo "--- :inbox_tray: installing deps"
  make deps-install
}

do_bins(){
  echo "--- :hammer_and_pick: building binaries"
  make bins
}

do_vet(){
  echo "--- :female-police-officer::skin-tone-4: vet"
  make test-vet
}

do_lint(){
  echo "--- :building_construction: installing lint deps"
  make lintdeps-install

  echo "--- :mag: linting"
  make test-lint || {
    echo "--- :rotating_light: excessive lint errors"
  }
}

do_tests(){
  echo "--- :female-scientist: runnig unit tests"
  make test-full
}

do_tests_lite(){
  echo "--- :female-scientist: runnig unit tests"
  make test
}

do_coverage(){
  echo "--- :female-scientist: capturing test coverage"
  go test -coverprofile=coverage.txt -covermode=count -coverpkg="./..." ./...

  echo "--- :satellite_antenna: uploading test coverage"
  bash <(curl -s https://codecov.io/bash)
}

do_integration(){
  echo "--- :building_construction: installing integration dependencies"
  make integrationdeps-install

  echo "--- :juggling: running integration tests"
  make test-integration
}

case "$1" in
  test)
    do_glide
    do_tests
    ;;
  test-lite)
    do_glide
    do_bins
    do_tests_lite
    ;;
  coverage)
    do_glide
    do_coverage
    ;;
  integration)
    do_glide
    do_integration
    ;;
  lint)
    do_glide
    do_bins
    do_vet
    do_lint
    ;;
esac
