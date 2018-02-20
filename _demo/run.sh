#!/bin/sh

CMD=$0

nodeport(){
  echo $(kubectl get service node-0-photon-node -o jsonpath='{.spec.ports[?(@.name == "photond-rpc")].nodePort}')
}

do_node(){
  echo "http://$(minikube ip):$(nodeport)"
}

do_send(){
  export PHOTON_NODE=$(do_node)
  ../photon send 100 $(cat data/other.key) -k master --node "$PHOTON_NODE" -d data/client
}

do_query(){
  key=${1:-master}
  export PHOTON_NODE=$(do_node)
  ../photon query $(cat "data/$key.key") -d data/client
}

do_usage(){
  echo "USAGE: $CMD <node|send|query>" >&2
  exit 1
}

cmd=$1
shift
case "$cmd" in
  node)
    do_node "$@"
    ;;
  send)
    do_send "$@"
    ;;
  query)
    do_query "$@"
    ;;
  *)
    do_usage
esac
