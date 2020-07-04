# Dev Environment: "Kube" configuration

The _Kube_ dev environment builds:

* A single-node blockchain network
* An Akash Provider Services Daemon (PSD) for bidding and running workloads.
* A Kubernetes cluster for the PSD to run workloads on.

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
|`PRICE`|10akash|price to bid|

# Runbook

The following steps will bring up a network and allow for interacting
with it.

Running through the entire runbook requires three terminals.
Each command is marked __t1__-__t3__ to indicate a suggested terminal number.

If at any time you'd like to start over with a fresh chain, simply run:

__t1 run__
```sh
make clean kind-cluster-clean 
make init
```

## Initialize

Start and initialize kind.

Kubernetes ingress objects present some difficulties for creating development
environments.  Two options are offered below - the first (random port) is less error-prone
and can have multiple instances run concurrently, while the second option arguably
has a better payoff.

**note**: this step waits for Kubernetes metrics to be available, which can take some time.
The counter on the left side of the messages is regularly in the 120 range.  If it goes beyond 250,
there may be a problem.


| Option  | __t1 Step: 1__ | Explanation  |
|---|---|---|
| Map random local port to port 80 of your workload | `make kind-cluster-create` | This is less error-prone, but makes it difficult to access your app through the browser. |
| Map localhost port 80 to workload  | `KIND_CONFIG=kind-config-80.yaml make kind-cluster-create` | If anything else is listening on port 80 (any other web server), this method will fail.  If it does succeed, you will be able to browse your app from the browser. |

## Build Akash binaries and initialize network

Initialize keys and accounts:

### __t1 Step: 2__
```sh
make init
```

## Run local network

In a separate terminal, the following command will run the `akashd` node:

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

## Run provider services

In a third terminal, run the Provider service with command:

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

And when the order is ready to be matched, a lease will be created:

```sh
make query-leases
```

You should now see "pending" inventory inventory in the provider status:

```sh
make provider-status
```

## Distribute Manifest

Now that you have a lease with a provider, you need to send your
workload configuration to that provider by sending it the manifest:

### __t1 Step: 7__
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
make provider-service-status
```

Fetch logs from deployed service (all pods)

__t1 service logs__
```sh
make provider-service-logs
```

If you chose to use port 80 when setting up kind, you can browse to your
deployed workload at http://hello.localhost

## Terminate lease

There are a number of ways that a lease can be terminated.

#### Provider closes the bid:

__t1 teardown__
```sh
make bid-close
```

#### Tenant closes the order

__t1 teardown__
```sh
make order-close
```

#### Tenant closes the deployment

__t1 teardown__
```sh
make deployment-close
```
