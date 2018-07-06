source ../common.sh

nodeuri(){
  echo "http://node-0.$(minikube ip).nip.io:80"
}

export AKASH_NODE=$(nodeuri)
export AKASH_DATA=$GOPATH/src/github.com/ovrclk/akash/_run/multi/data/client
