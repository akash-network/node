
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

Running through the entire suite requires three terminals.
Each command is marked __t1__-__t3__ to indicate a suggested terminal number.

* https://kubectl.docs.kubernetes.io/pages/reference/kustomize.html

## Setup

**Developer Deps**: You will need `kubectl` installed

### Overview

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

To get DNS routing to work locally, there are two addresses which will probably need to set to configure requests to hit the kind docker container. To route requests back to the local interface, add the following two lines to your `/etc/hosts` for the Akash-Node and Akash-Provider examples to work correctly.

* `127.0.0.1   akash.localhost`
* `127.0.0.1   akash-provider.localhost`

Or if it does not conflict with other local rules, use a wildcard for localhost:
* `127.0.0.1   *.localhost`

## Runbook

The following steps will bring up a network and allow for interacting
with it.

Running through the entire runbook requires two terminals.
Each command is marked __t1__-__t2__ to indicate a suggested terminal number.

If at any time you'd like to start over with a fresh chain, simply run:

__t1__
```sh
make clean kind-cluster-clean
```

### Initialize Cluster

Start and initialize kind. There are two options for network manager; standard CNI, or Calico.
Both are configured with Makefile targets as specified below. Using Calico enables testing of
Network Policies.

**note**: If anything else is listening on port 80 (any other web server), this will fail.

Pick one of the following commands:
__t1__
```sh
# Standard Networking
make kind-cluster-create

# Calico Network Manger
make kind-cluster-calico-create
```

Check all pods in kube-system and ingress-nginx namespaces are in Running state.
If some pods are in Pending stay give it a little wait and check again
```shell
kubectl --context kind-single -n ingress-nginx -n kube-system get pods
```

### (Optional) Upload a local docker image

If you specified a custom image in the earlier step you need to upload that image into the Kubernetes
cluster created by the `kind` command. This uploads an image from your local docker into the Kubernetes cluster.

__t1__
```sh
DOCKER_IMAGE=ovrclk/akash:mycustomtag make kind-upload-image
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
make kustomize-install-node
```

You can check the status of the network with:

__t1__
```sh
make node-status
```

You should see blocks being produced - the block height should be increasing.

You can now view genesis accounts that were created:

**If this command fails**, consider adding `127.0.0.1   akash.localhost` to your `/etc/hosts` for DNS.

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

__t2__
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

You should now see "pending" inventory inventory in the provider status:

__t1__
```sh
make provider-status
```

### Create a lease

Create a lease for the bid from the provider:

__t1__
```sh
make lease-create
```

You should be able to see the lease with

__t1__
```sh
make query-leases
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

### Withdraw from the lease

Withdraw some funds from the lease

__t1__
```sh
make lease-withdraw
```

You should be able to see the escrow payment change in

__t1__
```sh
make query-deployment
```

and

__t1__
```sh
make query-accounts
```

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

#### Limitations

Akash Groups are translated into Kubernetes Deployments, this means that only a few fields from the Akash SDL are mutable. For example `image`, `command`, `args`, `env` and exposed ports can be modified, but compute resources and placement criteria cannot.

### Terminate lease

There are a number of ways that a lease can be terminated.

#### Provider closes the bid:

__t1__
```sh
make lease-close
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
