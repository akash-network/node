
# Dev Environment: "Single" configuration

The _Single_ dev environment builds A single-node blockchain network and
an Akash Provider Services Daemon (PSD) for bidding and running workloads,
all within a kind kubernetes environment.

The [instructions](#runbook) below will illustrate how to:

* [Initialize blockchain node and client](#initialize)
* [Run a single-node network](#run-local-network)
* [Query objects on the network](#run-query)
* [Create a provider](#create-a-provider)
* [Run provider services](#run-provider-services)
* [Create a deployment](#create-a-deployment)
* [Bid on an order](#create-a-bid)
* [Terminate a lease](#terminate-lease)

See [commands](#commands) for a full list of utilities meant
for interacting with the network.

Run a network with a single, local node and execute workloads in Minikube.

Running through the entire suite requires four terminals.
Each command is marked __t1__-__t4__ to indicate a suggested terminal number.

* TODO: https://kind.sigs.k8s.io/docs/user/local-registry/

* https://kubectl.docs.kubernetes.io/pages/reference/kustomize.html

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

To get DNS routing to work locally, there are two addresses which will probably need to set to configure requests to hit the kind docker container. To route requests back to the local interface, add the following two lines to your `/etc/hosts` for the Akashd and Akash-Provider examples to work correctly.

* `127.0.0.1   akashd.localhost`
* `127.0.0.1   akash-provider.localhost`

Or if it does not conflict with other local rules, use a wildcard for localhost:
* `127.0.0.1   *.localhost`

## Runbook

The following steps will bring up a network and allow for interacting
with it.

Running through the entire runbook requires four terminals.
Each command is marked __t1__-__t3__ to indicate a suggested terminal number.

If at any time you'd like to start over with a fresh chain, simply run:

__t1__
```sh
make clean-all kind-cluster-clean
make init kustomize-init
```

### Initialize Cluster

Start and initialize kind. There are two options for network manager; standard CNI, or Calico.
Both are configured with Makefile targets as specified below. Using Calico enables testing of
Network Polcies.

**note**: this step waits for kubernetes metrics to be available, which can take some time.
The counter on the left side of the messages is regularly in the 120 range.  If it goes beyond 250,
there may be a problem.

**note**: If anything else is listening on port 80 (any other web server), this
will fail.

Pick one of the following commands:
__t1__
```sh
# Standard Networking
make kind-cluster-create

# Calico Network Manger
make kind-cluster-calico-create
```


### Build Akash binaries and initialize network

__t1__
```sh
make init
```

### Initialize kustomize

```sh
make kustomize-init
```

### Run local network

```sh
make kustomize-install-akashd
```

You can check the status of the network with:

__t1__
```sh
make node-status
```

You should see blocks being produced - the block height should be increasing.

You can now view genesis accounts that were created:

**If this command fails**, consider adding `127.0.0.1   akashd.localhost` to your `/etc/hosts` for DNS.

__t1__
```sh
make query-accounts
```

### Create a provider

Create a provider on the network with the following command:

__t1__
```sh
make provider-create
```

View the on-chain representation of the provider with:

__t1__
```sh
make query-provider
```

### Run provider services

In a separate terminal, run the following command

__t3__
```sh
make kustomize-install-provider
```

Query the provider service gateway for its status:

__t1__
```sh
make provider-status
```

### Create a deployment

Create a deployment from the `main` account with:

__t1__
```sh
make deployment-create
```

This particular deployment is created from the sdl file in this directory ([`deployment.yaml`](deployment.yaml)).

Check that the deployment was created.  Take note of the `dseq` - deployment sequence:

__t1__
```sh
make query-deployments
```

After a short time, you should see an order created for this deployment with the following command:

__t1__
```sh
make query-orders
```

The Provider Services Daemon should see this order and bid on it.

__t1__
```sh
make query-bids
```

And when the order is ready to be matched, a lease will be created:

__t1__
```sh
make query-leases
```

You should now see "pending" inventory inventory in the provider status:

__t1__
```sh
make provider-status
```

### Distribute Manifest

Now that you have a lease with a provider, you need to send your
workload configuration to that provider by sending it the manifest:

__t1__
```sh
make send-manifest
```

You can check the status of your deployment with:

__t1__
```sh
make provider-lease-status
```

You can reach your app with the following (Note: `Host:` header tomfoolery abound)
__t1__
```sh
make provider-lease-ping
```

If you chose to use port 80 when setting up kind, you can browse to your
deployed workload at http://hello.localhost

## Update Provider

If the KinD configuration uses Docker's random port assignment then the on-chain Provider data will need to be updated for `send-manfiest` to be able to correctly route the manifest POST request.

For example you might need to update the `provider.yaml`'s first line to include the port number. eg: `host: http://akash-provider.localhost:41109`


## Update Deployment

Updating active Deployments is a two step process. First edit the `deployment.yaml` with whatever changes are desired. Example; update the `image` field.
 1. Update the Akash Network to inform the Provider that a new Deployment declaration is expected.
   * `make deployment-update`
 2. Send the updated manifest to the Provider to run.
   * `make send-manifest`

Between the first and second step, the prior deployment's containers will continue to run until the new manifest file is received, validated, and new container group operational. After health checks on updated group are passing; the prior containers will be terminated.

### Terminate lease

There are a number of ways that a lease can be terminated.

#### Provider closes the bid:

__t1__
```sh
make bid-close
```

#### Tenant closes the order

__t1__
```sh
make order-close
```

#### Tenant closes the deployment

__t1__
```sh
make deployment-close
```
