# Akash: Single Node Testnet Setup

This testing harness sets up a [minikube](https://github.com/kubernetes/minikube) environment
running multiple Akash nodes and providers with [kubernetes](https://kubernetes.io/).

The [kubernetes](https://kubernetes.io/) configuration is managed by two [helm](https://helm.sh) 
charts: `akash-node` and `akash-provider`.

Running through the entire suite requires two terminals.
Each command is marked __t1__-__t2__ to indicate a suggested terminal number.

Logging of nodes and providers can easily be obtained by using [kail](https://github.com/boz/kail)

Example snippets for working with this environment can be found [below](#tinkering).

## Dependencies

Ensure that you have installed the base dependencies and have set `GOPATH` [as described here](https://github.com/ovrclk/akash). Then install these additional dependencies before continuing:

 * [docker](https://www.docker.com/community-edition#/download)
 * [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
 * [minikube](https://github.com/kubernetes/minikube)
 * [virtualbox](https://www.virtualbox.org/wiki/Downloads)
 * [helm](https://docs.helm.sh/using_helm/#installing-helm)
 * [kail](https://github.com/boz/kail) _(optional)_

## Setup

__t1__: Start minikube

```sh
make kube-start
```

## Initialize a new chain

You can run `make init` to perform the below set of steps. The below task create keys, add genesis accounts and create a validator.

```
make init
```

## Start minikube

```
make kube-start
```

# Install Nodes

```
make deploy
```

## Remove Node

```
make remove
```
