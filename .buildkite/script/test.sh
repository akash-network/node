#!/bin/bash

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
  echo "--- :mag: linting"
  make test-vet
}

do_tests(){
  echo "--- :female-scientist: runnig unit tests"
  # make test-full
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
    do_bins
    do_vet
    do_tests
    ;;
  coverage)
    do_glide
    do_coverage
    ;;
  integration)
    do_glide
    do_integration
    ;;
esac
