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
$ cd $GOPATH/src/github.com/ovrclk/akash/_run/multi
$ minikube start --cpus 4 --memory 4096
$ minikube addons enable ingress
$ minikube addons enable metrics-server
$ kubectl create -f rbac.yaml
```

__t1__: Initialize helm
```sh
$ helm init
```

__t1__: Build, push docker image into minikube

_Creates 6 providers: us-west-1, us-west-2, us-east-1, us-east-2, ap-southeast-1, ap-southeast-2_
```sh
$ make image-minikube 
```

__t1__: Generate genesis and config

_run.sh wraps various shell commands to make the prototype eaiser to run. The init command makes two wallets (named master and other), a genesis configuration giving master all the tokens, and configuration for four nodes located in data/node/_

```sh
$ make # make sure to compile latest akash binaries
$ ./run.sh init 
```

## Start Network

__t1__: Deploy Akash nodes
```sh
$ make helm-install-nodes
```

__t1__: Wait for blocks to be created

_Repeat `akash status` as needed until first block is created_
```sh
$ source env.sh
$ akash status 
```

## Transfer Tokens from Master Account to Other Account

__t1__: Query _master_ account

_Checks master's token balance_
```sh
$ ./run.sh query master
```

__t1__: Send tokens to _other_ account

_Sends 100 tokens to other_
```sh
$ ./run.sh send
```

__t1__: Query _master_ account

_Checks master's token balance to verify send (for demo purposes)_
```sh
$ ./run.sh query master
```

__t1__: Query _other_ account

_Checks other's token balance to verify receipt (for demo purposes)_
```sh
$ ./run.sh query other
```

## Marketplace

__t2__: Start marketplace monitor in terminal 2

_Starts marketplace and prints marketplace transaction log for visibility into the goings-on_
```sh
$ ./run.sh marketplace
```

__t1__: Run providers

_Creates providers (datacenters). Check the markeplace monitor to see them come online_
```sh
$ make helm-install-providers
```

__t1__: Create Deployment

_Creates a deployment for the master acct using the sample deployment.yaml file. Then:_
 * _orders are then created from deployments,_ 
 * _providers bid on them using fulfillments, which are printed in the format deployment-address/group-id/order-id/provider-address, along with bid price,_
 * _a lease is awarded to the lowest bid provider and printed_
 * _the manifest file is then automatically sent to the winning provider_
```sh
$ akash deployment create deployment.yaml -k master
```

__t1__: Check/View deployed app

_Quick check to see that the sample app has been automatically deployed, then take a look at the sample app_
```sh
$ curl -I hello.$(minikube ip).nip.io
$ open http://hello.$(minikube ip).nip.io
```

__t1__: Create, deploy, view a second app in a different region

_Copies deployment.yaml to world.yaml, replacing the "hello" subdomains with "world" subdomains and the us-west region with ap-southeast. Then sends the deployment and checks the sample app as before_
```sh
$ source env.sh
$ sed -e 's/hello/world/g' -e 's/us-west/ap-southeast/g' -e 's/westcoast/singapore/g'< deployment.yaml > world.yaml
$ akash deployment create world.yaml -k master
$ curl -I world.$(minikube ip).nip.io
$ open http://world.$(minikube ip).nip.io
```

## Shutdown
__t1__: Delete minikube

_Deletes minikube to conserve resources on your local machine_
```sh
$ minikube delete
```

__t2__: Shut down marketplace

_Please do not judge our lack of elegance. It's a prototype_
```sh
$ ^c
```


# Tinkering 

## Kubernetes/Helm

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

Create/upgrade/delete/reset a single node (`node-0`)
```sh
$ make helm-install-node-node-0
$ make helm-upgrade-node-node-0
$ make helm-delete-node-node-0
$ make helm-reset-node-node-0
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

## Akash API
**Note that you must load shell helpers as shown below into any terminal in which you wish to run `akash` commands**

Load shell helpers
```sh
$ source env.sh
# Needed to run akash commands
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
