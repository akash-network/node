# Tutorial: "Lite" configuration

This example showcases Akash blockchain operations with a
single-node network.

The [instructions](#runbook) below will illustrate how to:

* [Initialize blockchain node and client](#initialize)
* [Run a single-node network](#run-local-network)
* [Query objects on the network](#run-query)
* [Create a provider](#create-a-provider)
* [Run provider services](#run-provider-services) _(optional)_
* [Create a deployment](#create-a-deployment)
* [Bid on an order](#create-a-bid)
* [Terminate a lease](#terminate-lease)

See [commands](#commands) for a full list of utilities meant
for interacting with the network.

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

## Runbook

The following steps will bring up a network and allow for interacting
with it.

If at any time you'd like to start over with a fresh chain, simply run:

```sh
make clean init
```

### Initialize

The following command will

* build `akashd` and `akashctl`
* create configuration directories
* create a genesis file with [accounts](#setup) and single validator.

```sh
make init
```

### Run local network

In a separate terminal, the following command will run the `akashd` node:
```sh
make node-run
```

You can check the status of the network with:
```sh
make node-status
```

You should see blocks being produced - the block height should be increasing.

You can now view genesis accounts that were created:

```sh
make query-accounts
```

### Create a provider

Create a provider on the network with the following command:
```sh
make provider-create
```

View the on-chain representation of the provider with:
```sh
make query-provider
```

### Run provider services

__NOTE__: running a provider is optional.  If you want to bid on orders
yourself, skip this step.

In a separate terminal, run the following command

```sh
make provider-run
```

Query the provider service gateway for its status:

```sh
make provider-status
```

### Create a deployment

Create a deployment from the `main` account with:

```sh
make deployment-create
```

This particular deployment is created from the sdl file in this directory ([`deployment.yaml`](deployment.yaml)).

Check that the deployment was created.  Take note of the `dseq` - deployment sequence:
```sh
make query-deployments
```

After a short time, you should see an order created for this deployment with the following command:
```sh
make query-orders
```

### Create a bid

__NOTE__: if you are [running provider services](#run-provider-services), skip the first step here - it is handled
by the provider daemon.

Create a bid for the order from the provider:

```sh
make bid-create
```

You should be able to see the bid with

```sh
make query-bid
```

Eventually a lease will be generated.  You can see it with:

```sh
make query-leases
```


_if_ you are running provider services, query the provider gateway:
```sh
make provider-status
```

### Terminate lease

There are a number of ways that a lease can be terminated.

#### Provider closes the bid:

```sh
make bid-close
```

#### Tenant closes the order

```sh
make order-close
```

#### Tenant closes the deployment

```sh
make deployment-close
```

## Commands

* [Querying](#querying)
  * [Accounts](#accounts)
  * [Deployments](#deployments)
  * [Orders](#orders)
  * [Bids](#bids)
  * [Leases](#leases)
  * [Providers](#providers)
* [Transactions](#transactions)
  * [Deployments](#deployments-1)
  * [Orders](#orders-1)
  * [Bids](#bids-1)
  * [Providers](#providers-1)

### Querying

Query commands fetch and display information from the blockchain.

#### Accounts

Query all accounts:

```sh
make query-accounts
```

Query individual accounts:
```sh
make query-account-main
make query-account-provider
make query-account-validator
make query-account-other
```

### Deployments

Query all deployments:

```sh
make query-deployments
```

Query a single deployment:

```sh
DSEQ=4 make query-deployments
```

### Orders

Query all orders:

```sh
make query-orders
```

Query a single order:

```sh
DSEQ=4 GSEQ=1 OSEQ=1 make query-deployment
```

### Bids

Query all bids:

```sh
make query-bids
```

Query a single bid:

```sh
DSEQ=4 GSEQ=1 OSEQ=1 make query-bid
```

### Leases

Query all leases:

```sh
make query-leases
```

Query a single lease:

```sh
DSEQ=4 GSEQ=1 OSEQ=1 make query-lease
```

### Providers

Query all providers:
```sh
make query-providers
```

Query a single provider:
```sh
PROVIDER_KEY_NAME=validator make query-provider
```

### Transactions

Transaction commands modify blockchain state.

#### Deployments

Create a deployment with `DSEQ` set to the current block height
```sh
DSEQ=0 make deployment-create
```

Fully-customized deployment creation:
```sh
SDL_PATH=yolo.yaml DSEQ=20 KEY_NAME=other make deployment-create
```

Close a deployment with a custom `DSEQ`
```sh
DSEQ=10 make deployment-close
```

#### Orders

Close an order with the default parameters
```sh
make order-close
```

Fully-customized order close
```sh
KEY_NAME=other DSEQ=20 GSEQ=99 OSEQ=500 make order-close
```

#### Bids

Fully-customized bid creation
```sh
KEY_NAME=other PROVIDER_KEY_NAME=validator DSEQ=20 GSEQ=99 OSEQ=500 PRICE=100akash make bid-create
```

Fully-customized bid close
```sh
KEY_NAME=other PROVIDER_KEY_NAME=validator DSEQ=20 GSEQ=99 OSEQ=500 make bid-close
```

#### Providers

Fully-customized provider creation
```sh
PROVIDER_KEY_NAME=validator PROVIDER_CONFIG_PATH=rogue.yaml make provider-create
```

