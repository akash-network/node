# Akash Demo Commands

- [Akash Demo Commands](#akash-demo-commands)
  * [Build](#build)
  * [Initialize data directory](#initialize-data-directory)
    + [Node](#node)
    + [Client](#client)
  * [Start node](#start-node)
  * [Create account](#create-account)
  * [Query account](#query-account)
  * [Send tokens](#send-tokens)
  * [Create deployment](#create-deployment)
  * [Query deployment](#query-deployment)
      - [Query all deployments](#query-all-deployments)
- [Multi node setup](#multi-node-setup)
  * [Create client keys](#create-client-keys)
  * [Setup validater nodes](#setup-validater-nodes)
    + [Initalize validater nodes](#initalize-validater-nodes)
    + [genesis.json](#genesisjson)
    + [config.toml](#configtoml)
      - [Change port numbers (If running on the same IP/domain)](#change-port-numbers--if-running-on-the-same-ip-domain-)
    + [Add seeds to config](#add-seeds-to-config)
  * [Setup non-validater node](#setup-non-validater-node)
    + [Initalize validater nodes](#initalize-validater-nodes-1)
    + [genesis.json](#genesisjson-1)
    + [config.toml](#configtoml-1)
    + [Add seeds to config](#add-seeds-to-config-1)
  * [Start nodes](#start-nodes)
- [Third party library reference:](#third-party-library-reference-)
    + [Accounts](#accounts)
    + [Node configuration](#node-configuration)
  * [notes](#notes)

## Build

```sh
make
```

## Initialize data directory

### Node

```sh
./akashd init <public key>
```

### Client

```sh
./akash init --node=tcp://<url>:<port> --genesis=<path/to/genesis.json>
```

example:

```sh
./akash init --node=tcp://localhost:26657 --genesis=data/node/genesis.json
```

## Start node

```sh
./akashd start
```

## Create account

```sh
./akash key create <account name>
```

## Query account

```sh
./akash query account <public key>
```

## Send tokens

```sh
./akash send <amount> <to address> -k <account name> [flags]
```

## Create deployment
```
./akash deploy <filepath> -k <account name>
```

Returns the 32 byte address of the deployment

## Query deployment
```
./akash query deployment [address]
```

Returns deployment object located at [address]

#### Query all deployments
```
./akash query deployment
```

Returns all deployment objects

## Create provider
```
./akash provider <filepath> -k <account name>
```

Returns the 32 byte address of the provider
Note: If no account name exists it will be created

## Query provider
```
./akash query provider [address]
```

Returns provider object located at [address]

#### Query all provider
```
./akash query provider
```

Returns all provider objects


# Multi node setup

## Create client keys

```sh
./client keys new issuer
```

## Setup validater nodes

### Initalize validater nodes

```sh
./node init <issuer public key> --home=./data/node1 --chain-id=akash-test
./node init <issuer public key> --home=./data/node2 --chain-id=akash-test
./node init <issuer public key> --home=./data/node3 --chain-id=akash-test
./node init <issuer public key> --home=./data/node4 --chain-id=akash-test
```

### genesis.json

The public keys of node1, node2, node3, and node4 must be present in each's respective genesis file.

`Genesis file location: ./data/node*/genesis.json`

In the genesis file for a single node, the `"validators"` array initally contains the node's public key.
Copy and paste these array members into each nodes `"validators"` array so that there are three unique entries in each geneis file.

### config.toml

The config.toml defines the locations of the node's own services and the locations of other nodes in the network.
Each node must have entries for each validator node.

#### Change port numbers (If running on the same IP/domain)

Afer initializtion of a nodes their config.toml contain duplicate values.
For node2, node3, and node4 increment each fields ports by some factor of 10

Fields:
* proxy_app
* [rpc]laddr
* [p2p]laddr

Example:

node2
```
laddr = "tcp://0.0.0.0:46667"
```

node3
```
laddr = "tcp://0.0.0.0:46677"
```

### Add seeds to config

For the `[p2p] seeds` field add each validators `[rpc]laddr` separated by comma, ommiting a validator node's own address.

Example:
```
seeds = "0.0.0.0:46666,0.0.0.0:46676,0.0.0.0:46686"
```

## Setup non-validater node

The non-validator will not vote on blocks but will verify and keep up with the consensus protocol.

### Initalize validater nodes

```sh
./node init <issuer public key> --home=./data/node5 --chain-id=akash-test
```

### genesis.json

Copy the genesis.json from one of the validator nodes to this node.

### config.toml

### Add seeds to config

For the `[p2p] seeds` field add each validators `[p2p]laddr` separated by comma

Example:
```
"0.0.0.0:26656,0.0.0.0:26666,0.0.0.0:26676,0.0.0.0:26686"
```

## Start nodes

In separate temainal sessions or environments

```sh
./node start --home=data/node1
./node start --home=data/node2
./node start --home=data/node3
./node start --home=data/node4
./node start --home=data/node5
```

# Third party library reference:

## Node configuration
https://github.com/cosmos/cosmos-sdk/blob/master/docs/staking/local-testnet.rst
https://github.com/tendermint/tendermint/blob/master/docs/using-tendermint.rst

## notes

```sh
export AKASH_DATA=$PWD/devdata/client
export AKASHD_DATA=$PWD/devdata/node

rm -rf $AKASH_DATA
MASTER_ADDRESS=$(./akash key create master)

rm -rf $AKASHD_DATA
./akashd init $MASTER_ADDRESS
./akashd start

USER_ADDRESS=$(./akash key create user)
./akash send 100 $USER_ADDRESS -k master
```
