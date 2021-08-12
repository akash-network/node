# Dev Environment: "Minikube" configuration

The _Minikube_ dev environment builds:

* A single-node blockchain network
* An Akash Provider Services Daemon (PSD) for bidding and running workloads.
* A Kubernetes cluster managed by **minikube** for the PSD to run workloads on.

The [instructions](#runbook) below will illustrate how to run a network with a single, local node and execute workloads in [kind](https://kind.sigs.k8s.io/):

* [Initialize blockchain node and client](#initialize)
* [Run a single-node network](#run-local-network)
* [Query objects on the network](#run-query)
* [Create a provider](#create-a-provider)
* [Run provider services](#run-provider-services)
* [Create a deployment](#create-a-deployment)
* [Bid on an order](#create-a-bid)
* [Terminate a lease](#terminate-lease)

## Setup

Four keys and accounts are created.  The key names are:

|Key Name|Use|
|---|---|
|`main`|Primary account (creating deployments, etc...)|
|`provider`|The provider account (bidding on orders, etc...)|
|`validator`|The sole validator for the created network|
|`other`|Misc. account to (receives tokens, etc...)|

Most `make` commands are configurable and have defaults to make it
such that you don't need to override them for a simple pass-through of
this example.

|Name|Default|Description|
|---|---|---|
|`KEY_NAME`|`main`|standard key name|
|`PROVIDER_KEY_NAME`|`provider`|name of key to use for provider|
|`DSEQ`|1|deployment sequence|
|`GSEQ`|1|group sequence|
|`OSEQ`|1|order sequence|
|`PRICE`|10uakt|price to bid|

# Runbook

The following steps will bring up a network and allow for interacting
with it.

Running through the entire runbook requires three terminals.
Each command is marked __t1__-__t3__ to indicate a suggested terminal number.

If at any time you'd like to start over with a fresh chain, simply run:

__t1 run__
```sh
make clean minikube-cluster-delete
make init
```

## Initialize

Start and initialize minikube.

If MINIKUBE_VM_DRIVER variable is empty script is trying to determine available hypervisors
### MacOS:
    
### Linux

```sh
make minikube-cluster-create
```

## Build Akash binaries and initialize network

Initialize keys and accounts:

### __t1 Step: 2__
```sh
make init
```

## Run local network

In a separate terminal, the following command will run the `akash` node:

### __t2 Step: 3__
```sh
make node-run
```

You can check the status of the network with:

__t1 status__
```sh
make node-status
```

You should see blocks being produced - the block height should be increasing.

You can now view genesis accounts that were created:

__t1 status__
```sh
make query-accounts
```

## Create a provider

Create a provider on the network with the following command:

### __t1 Step: 4__
```sh
make provider-create
```

View the on-chain representation of the provider with:

__t1 status__
```sh
make query-provider
```

## Run Provider

To run Provider as a simple binary connecting to the cluster, in a third terminal, run the command:

### __t3 Step: 5__
```sh
make provider-run
```

Query the provider service gateway for its status:

__t1 status__
```sh
make provider-status
```

## Create a deployment

Create a deployment from the `main` account with:

### __t1 run Step: 6__
```sh
make deployment-create
```

This particular deployment is created from the sdl file in this directory ([`deployment.yaml`](deployment.yaml)).

Check that the deployment was created.  Take note of the `dseq` - deployment sequence:

__t1 status__
```sh
make query-deployments
```

After a short time, you should see an order created for this deployment with the following command:

```sh
make query-orders
```

The Provider Services Daemon should see this order and bid on it.

```sh
make query-bids
```

When a bid has been created, you may create a lease:


### __t1 run Step: 7__

To create a lease, run

```sh
make lease-create
```

You can see the lease with:

```sh
make query-leases
```

You should now see "pending" inventory in the provider status:

```sh
make provider-status
```

## Distribute Manifest

Now that you have a lease with a provider, you need to send your
workload configuration to that provider by sending it the manifest:

### __t1 Step: 8__
```sh
make send-manifest
```

You can check the status of your deployment with:

__t1 status__
```sh
make provider-lease-status
```

You can reach your app with the following (Note: `Host:` header tomfoolery abound)

__t1 status__
```sh
make provider-lease-ping
```

Get service status

__t1 service status__
```sh
make provider-lease-status
```

Fetch logs from deployed service (all pods)

__t1 service logs__
```sh
make provider-lease-logs
```

If you chose to use port 80 when setting up kind, you can browse to your
deployed workload at http://hello.localhost

## Update Deployment

Updating active Deployments is a two step process. First edit the `deployment.yaml` with whatever changes are desired. Example; update the `image` field.
 1. Update the Akash Network to inform the Provider that a new Deployment declaration is expected.
   * `make deployment-update`
 2. Send the updated manifest to the Provider to run.
   * `make send-manifest`

Between the first and second step, the prior deployment's containers will continue to run until the new manifest file is received, validated, and new container group operational. After health checks on updated group are passing; the prior containers will be terminated.

#### Limitations

Akash Groups are translated into Kubernetes Deployments, this means that only a few fields from the Akash SDL are mutable. For example `image`, `command`, `args`, `env` and exposed ports can be modified, but compute resources and placement criteria cannot.

## Terminate lease

There are a number of ways that a lease can be terminated.

#### Provider closes the bid:

__t1 teardown__
```sh
make bid-close
```

#### Tenant closes the lease

__t1 teardown__
```sh
make lease-close
```

#### Tenant pauses the group

__t1 teardown__
```sh
make group-pause
```

#### Tenant closes the group

__t1 teardown__
```sh
make group-pause
```

#### Tenant closes the deployment

__t1 teardown__
```sh
make deployment-close
```
