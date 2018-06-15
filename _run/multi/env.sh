source ../common.sh

nodeuri(){
  echo "http://node-0.$(minikube ip).nip.io:80"
}

akash(){
  _akash -n "$(nodeuri)" "$@"
}

akashd(){
  _akashd "$@"
}
