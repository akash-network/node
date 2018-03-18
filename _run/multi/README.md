# Akash: Multi-Node Local Setup

This testing harness sets up a [minikube](https://github.com/kubernetes/minikube) environment
running multiple Akash nodes and providers with [kubernetes](https://kubernetes.io/).

The [kubernetes](https://kubernetes.io/) configuration is managed by two [helm](https://helm.sh) 
charts: `akash-node` and `akash-provider`.

Running through the entire suite requires two terminals.
Each command is marked __t1__-__t2__ to indicate a suggested terminal number.

Logging of nodes and providers can easily be obtained by using [kail](https://github.com/boz/kail)

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
$ make helm-install
```

__t1__: Wait for blocks to be created
```sh
$ source ./env.sh
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
$ make helm-install-provider
```

__t1__: Create Deployment
```sh
$ ./run.sh deploy
```
