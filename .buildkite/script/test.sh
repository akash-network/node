#!/bin/bash

set -euo pipefail

do_deps(){
  echo "--- :inbox_tray: installing deps"
  make deps-install
}

do_bins(){
  echo "--- :hammer_and_pick: building binaries"
  make bins
}

do_image_bins(){
  echo "--- :hammer_and_pick: building image binaries"
  make image-bins
}

do_vet(){
  echo "--- :female-police-officer::skin-tone-4: vet"
  make test-vet
}

do_lint(){
  echo "--- :rotating_light: lint disabled.  see #360"

  # echo "--- :building_construction: installing lint deps"
  # make lintdeps-install

  # echo "--- :mag: linting"
  # make test-lint || {
  #   echo "--- :rotating_light: excessive lint errors"
  # }
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
  echo "--- :juggling: running integration tests"
  make test-integration
}

case "$1" in
  test)
    do_deps
    do_tests
    ;;
  test-lite)
    do_deps
    do_bins
    do_tests_lite
    ;;
  coverage)
    do_deps
    do_coverage
    ;;
  integration)
    do_deps
    do_integration
    ;;
  lint)
    do_deps
    do_bins
    do_image_bins
    do_vet
    do_lint
    ;;
esac
