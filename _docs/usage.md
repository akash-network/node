# Photon Demo Commands

- [Photon Demo Commands](#photon-demo-commands)
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
./photond init <public key>
```

### Client

```sh
./photon init --node=tcp://<url>:<port> --genesis=<path/to/genesis.json>
```

example:

```sh
./photon init --node=tcp://localhost:46657 --genesis=data/node/genesis.json
```

## Start node

```sh
./photond start
```

## Create account

```sh
./photon key create <account name>
```

## Query account

```sh
./photon query account <public key>
```

## Send tokens

```sh
./photon send <amount> <to address> -k <account name> [flags]
```

## Create deployment
```
./photon deploy <filepath> -k <account name>
```

Returns the 32 byte address of the deployment

## Query deployment
```
./photon query deployment [address]
```

Returns deployment object located at [address]

#### Query all deployments
```
./photon query deployment
```

Returns all deployment objects

## Create provider
```
./photon provider <filepath> -k <account name>
```

Returns the 32 byte address of the provider
Note: If no account name exists it will be created

## Query provider
```
./photon query provider [address]
```

Returns provider object located at [address]

#### Query all provider
```
./photon query provider
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
./node init <issuer public key> --home=./data/node1 --chain-id=photon-test
./node init <issuer public key> --home=./data/node2 --chain-id=photon-test
./node init <issuer public key> --home=./data/node3 --chain-id=photon-test
./node init <issuer public key> --home=./data/node4 --chain-id=photon-test
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
./node init <issuer public key> --home=./data/node5 --chain-id=photon-test
```


### genesis.json

Copy the genesis.json from one of the validator nodes to this node.

### config.toml

### Add seeds to config

For the `[p2p] seeds` field add each validators `[p2p]laddr` separated by comma

Example:
```
"0.0.0.0:46656,0.0.0.0:46666,0.0.0.0:46676,0.0.0.0:46686"
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

### Accounts

http://cosmos-sdk.readthedocs.io/en/latest/basecoin-basics.html
http://cosmos-sdk.readthedocs.io/en/latest/basecoin-tool.html

### Node configuration
https://github.com/cosmos/cosmos-sdk/blob/master/docs/staking/local-testnet.rst
https://github.com/tendermint/tendermint/blob/master/docs/using-tendermint.rst

## notes

```sh
export PHOTON_DATA=$PWD/devdata/client
export PHOTOND_DATA=$PWD/devdata/node

rm -rf $PHOTON_DATA
MASTER_ADDRESS=$(./photon key create master)

rm -rf $PHOTOND_DATA
./photond init $MASTER_ADDRESS
./photond start

USER_ADDRESS=$(./photon key create user)
./photon send 100 $USER_ADDRESS -k master
```
