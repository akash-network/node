#!/bin/sh
#shellcheck disable=SC2039
source ../common.sh

nodeuri(){
  echo "http://node-0.$(minikube ip).nip.io:80"
}

akash(){
  AKASH_NODE=$(nodeuri) _akash "$@"
}

akashd(){
  _akashd "$@"
}
