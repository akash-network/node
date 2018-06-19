# Akash: Multi-Node Local Setup

This testing harness sets up a [minikube](https://github.com/kubernetes/minikube) environment
running multiple Akash nodes and providers with [kubernetes](https://kubernetes.io/).

The [kubernetes](https://kubernetes.io/) configuration is managed by two [helm](https://helm.sh) 
charts: `akash-node` and `akash-provider`.

Running through the entire suite requires two terminals.
Each command is marked __t1__-__t2__ to indicate a suggested terminal number.

Logging of nodes and providers can easily be obtained by using [kail](https://github.com/boz/kail)

Example snippets for working with this environment can be found [below](#tinkering).

## Dependencies

Install the following dependencies before continuing:

 * [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
 * [minikube](https://github.com/kubernetes/minikube)
 * [helm](https://docs.helm.sh/using_helm/#installing-helm)
 * [kail](https://github.com/boz/kail) _(optional)_

## Setup

__t1__: Start minikube

```sh
$ minikube start --cpus 4 --memory 4096
$ minikube addons enable ingress
$ kubectl create -f rbac.yml
```

__t1__: Initialize helm
```sh
$ helm init
```

__t1__: Build, push docker image into minikube
```sh
$ make image-minikube
```

__t1__: Generate genesis and config
```sh
$ ./run.sh init
```

## Start Network

__t1__: Deploy Akash nodes
```sh
$ make helm-install-nodes
```

__t1__: Wait for blocks to be created
```sh
$ source env.sh
$ akash status # repeat until first block created (~45 seconds)
```

## Transfer Tokens

__t1__: Query _master_ account
```sh
$ ./run.sh query master
```

__t1__: Send tokens to _other_ account
```sh
$ ./run.sh send
```

__t1__: Query _master_ account
```sh
$ ./run.sh query master
```

__t1__: Query _other_ account
```sh
$ ./run.sh query other
```

## Marketplace

__t2__: Start marketplace monitor
```sh
$ ./run.sh marketplace
```

__t1__: Run providers
```sh
$ make helm-install-providers
```

__t1__: Create Deployment
```sh
$ ./run.sh deploy
```

__t1__: Check/View deployed app
```sh
$ curl -I hello.$(minikube ip).nip.io
$ open http://hello.$(minikube ip).nip.io
```

__t1__: Create, deploy, view a second app
```sh
$ source env.sh
$ sed -e 's/hello/world/g' < deployment.yml > world.yml
$ akash deployment create world.yml -k master -w
$ curl -I world.$(minikube ip).nip.io
$ open http://world.$(minikube ip).nip.io
```

# Tinkering

Get logs from all deployments
```sh
$ kail -l akash.network=true
```

Get logs from all nodes
```sh
$ kail -l akash.network/component=akashd
```

Get logs from all providers
```sh
$ kail -l akash.network/component=provider
```

Check status of node `node-0`:
```sh
$ make helm-check-node-node-0
```

Check status of all nodes:
```sh
$ make helm-check-nodes
```

Create/upgrade/delete/reset another node (`node-5`)
```sh
$ make helm-install-node-node-5
$ make helm-upgrade-node-node-5
$ make helm-delete-node-node-5
$ make helm-reset-node-node-5
```

Check status of provider `us-west-1`:
```sh
$ make helm-check-provider-us-west-1
$ curl us-west-1.$(minikube ip).nip.io/status
```

Check status of all providers:
```sh
$ make helm-check-providers
```

Create/upgrade/delete/reset a provider in region `us-central` (name: `us-central-1`)
```sh
$ make helm-install-provider-us-central-1
$ make helm-upgrade-provider-us-central-1
$ make helm-delete-provider-us-central-1
$ make helm-reset-provider-us-central-1
```

Load shell helpers
```sh
$ source env.sh
```

List deployments
```sh
$ akash query deployment
```

Other list options
```sh
$ akash query -h
```

Close deployment
```sh
$ akash deployment close <deployment-id> -k master
```

Close all active deployments
```sh
$ akash query deployment | \
  jq -r '.items[]|select(has("state")|not)|.address' | \
  while read id; do
    echo "closing deployment $id..."
    akash deployment close "$id" -k master
  done
```
