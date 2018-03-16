#!/bin/sh
# vim: ts=4 sts=4 sw=4 et

CMD=$0
DATA=$(dirname $0)/data/client

nodeport(){
  echo $(kubectl get service node-0-akash-node -o jsonpath='{.spec.ports[?(@.name == "akashd-rpc")].nodePort}')
}

do_node(){
  echo "http://$(minikube ip):$(nodeport)"
}

akash() {
  node=$(do_node)
  export AKASH_NODE=$node
  ../akash -n "$node" -d "$DATA" "$@"
}

do_send(){
  akash send 100 $(cat data/other.key) -k master
}

do_query(){
  key=${1:-master}
  akash query account $(cat "data/$key.key")
}

do_status(){
  akash status
}

do_monitor(){
  akash marketplace
}

do_provider(){
  akash provider create unused.yml -k dc
}

do_deploy(){
  akash deploy unused.yml -k dc
}

do_usage(){
  echo "USAGE: $CMD <node|send|query|status|monitor|provider|deploy>" >&2
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
  status)
    do_status "$@"
    ;;
  monitor)
    do_monitor "$@"
    ;;
  provider)
    do_provider "$@"
    ;;
  deploy)
    do_deploy "$@"
    ;;
  *)
    do_usage
    ;;
esac
