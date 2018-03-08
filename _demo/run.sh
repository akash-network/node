#!/bin/sh
# vim: ts=4 sts=4 sw=4 et

CMD=$0
DATA=$(dirname $0)/data/client

nodeport(){
  echo $(kubectl get service node-0-photon-node -o jsonpath='{.spec.ports[?(@.name == "photond-rpc")].nodePort}')
}

do_node(){
  echo "http://$(minikube ip):$(nodeport)"
}

do_send(){
  node=$(do_node)
  export PHOTON_NODE=$node
  ../photon send 100 $(cat data/other.key) -k master -n "$node" -d "$DATA"
}

do_query(){
  key=${1:-master}
  node=$(do_node)
  export PHOTON_NODE=$node
  ../photon query account $(cat "data/$key.key") -n "$node" -d "$DATA"
}

do_ping(){
  node=$(do_node)
  export PHOTON_NODE=$node
  ../photon ping -n "$node" -d "$DATA"
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
  ping)
    do_ping "$@"
    ;;
  *)
    do_usage
    ;;
esac
