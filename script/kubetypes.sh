#!/bin/bash
set -xe

pushd .cache
mkdir -p kubetypes_gopath
export GO111MODULE=on
export GOPATH=$PWD/kubetypes_gopath
ver=v0.19.3
GOMOD="$GOPATH/throwaway.go.mod"
echo 'module github.com/ovrclk/akash' > "$GOMOD"
go get -modcacherw -modfile "$GOMOD" -d "k8s.io/code-generator@${ver?}"

mkdir -p "$GOPATH/src"
pushd "$GOPATH/src"
gitremote="https://github.com/kubernetes/code-generator.git"
if [ ! -d "k8s.io/code-generator" ]; then
  git clone -b "${ver?}" "${gitremote?}" "k8s.io/code-generator"
fi
popd

script="pkg/mod/k8s.io/code-generator@${ver?}/generate-groups.sh"
chmod a+x "$GOPATH/${script?}"
exec "$GOPATH/${script?}" all \
	github.com/ovrclk/akash/pkg/client github.com/ovrclk/akash/pkg/apis \
	akash.network:v1
