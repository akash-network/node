source ../common.sh

nodeuri(){
  echo "http://node-0.$(minikube ip).nip.io:80"
}

akash(){
  export AKASH_NODE=$(nodeuri)
  _akash "$@"
}

akashd(){
  _akashd "$@"
}
