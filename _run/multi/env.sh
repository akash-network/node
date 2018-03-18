source ../common.sh

nodeport(){
  nodeno=${1:-0}
  echo $(kubectl get service node-${nodeno}-akash-node -o jsonpath='{.spec.ports[?(@.name == "akashd-rpc")].nodePort}')
}

nodeuri(){
  port=$(nodeport "$@")
  echo "http://$(minikube ip):$port"
}

akash(){
  _akash -n "$(nodeuri)" "$@"
}

akashd(){
  _akashd "$@"
}
