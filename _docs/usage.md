# Photon Demo Commands

- [Photon Demo Commands](#photon-demo-commands)
  * [Build](#build)
  * [Photon account commands (to be mered with basecoin)](#photon-account-commands--to-be-mered-with-basecoin-)
  * [Create basecoin account](#create-basecoin-account)
  * [Initialize data directory](#initialize-data-directory)
    + [Node](#node)
    + [Client](#client)
  * [Start node](#start-node)
  * [Query account](#query-account)
  * [Send tokens](#send-tokens)
- [Multi node setup](#multi-node-setup)
  * [Create client keys](#create-client-keys)
  * [Initalize validater nodes](#initalize-validater-nodes)
  * [Setup validater nodes](#setup-validater-nodes)
    + [genesis.json](#genesisjson)
    + [config.toml](#configtoml)
      - [Change port numbers (If running on the same IP/domain)](#change-port-numbers--if-running-on-the-same-ip-domain-)
    + [Add seeds to config](#add-seeds-to-config)
  * [Setup non-validater node](#setup-non-validater-node)
    + [genesis.json](#genesisjson-1)
    + [config.toml](#configtoml-1)
    + [Add seeds to config](#add-seeds-to-config-1)
  * [Start nodes](#start-nodes)
- [Reference for usage:](#reference-for-usage-)
    + [Accounts](#accounts)
    + [Node configuration](#node-configuration)

## Build

```sh
make
```

## Photon account commands (to be mered with basecoin)

<a href="../_docs/accounts.md">Photon account commands</a>

## Create basecoin account

```sh
<<<<<<< HEAD:_docs/usage.md
make build
./photon keys new cool
./photon keys new friend
=======
./client keys new <account name>
>>>>>>> redo redme for multinode setup:demo/README.md
```

## Initialize data directory

### Node

```sh
./node init <public key>
```

### Client

```sh
./client init --node=tcp://<url>:<port> --genesis=<path/to/genesis.json>
```

example:

```sh
./client init --node=tcp://localhost:46657 --genesis=data/node/genesis.json
```

## Start node

```sh
<<<<<<< HEAD:_docs/usage.md
./photond init <the address you copied>
./photond start
=======
./node start
>>>>>>> redo redme for multinode setup:demo/README.md
```

## Query account

```sh
./client query account <public key>
```

## Send tokens

```sh
<<<<<<< HEAD:_docs/usage.md
./photon init --node=tcp://localhost:46657 --genesis=$HOME/.demonode/genesis.json
=======
./client tx send --name=<your account name> --amount=<amount><denom> --to=<public key> --sequence=<sqn number>
>>>>>>> redo redme for multinode setup:demo/README.md
```

# Multi node setup

## Create client keys

```sh
<<<<<<< HEAD:_docs/usage.md
ME=$(./photon keys get cool | awk '{print $2}')
YOU=$(./photon keys get friend | awk '{print $2}')
./photon query account $ME
=======
./client keys new issuer
>>>>>>> redo redme for multinode setup:demo/README.md
```

## Setup validater nodes

### Initalize validater nodes

```sh
<<<<<<< HEAD:_docs/usage.md
./photon tx send --name=cool --amount=1000mycoin --to=$YOU --sequence=1
./photon query account $YOU
=======
./node init <issuer public key> --home=./data/node1 --chain-id=photon-test
./node init <issuer public key> --home=./data/node2 --chain-id=photon-test
./node init <issuer public key> --home=./data/node3 --chain-id=photon-test
<<<<<<< HEAD:_docs/usage.md
./node init <issuer public key> --home=./data/node3 --chain-id=photon-test
>>>>>>> redo redme for multinode setup:demo/README.md
=======
./node init <issuer public key> --home=./data/node4 --chain-id=photon-test
>>>>>>> fix readme:demo/README.md
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
