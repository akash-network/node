# Akash: Single-Node Local Setup with Minikube

Run a network with a single, local node and execute workloads in Minikube.

Running through the entire suite requires four terminals.
Each command is marked __t1__-__t4__ to indicate a suggested terminal number.

## Setup

__t1__: Start and initialize minikube
```sh
$ make minikube
$ minikube addons enable ingress
$ minikube addons enable metrics-server
```

__t1__: Build binaries
```sh
$ make bins
```

__t1__: Initialize environment
```sh
$ make init
```
Creates various accounts, genesis file, configuration, and account keys stored in the `test` keyring.

## Start Network

__t2__: Run `akashd`
```sh
$ make run-daemon
```

```sh
make provider
```

```sh
make run-provider
```

```sh
make provider-status
```

```sh
make deploy
```

```sh
make provider-status
```

```
 ../../akashctl --home cache/client provider send-manifest deployment.yaml --owner "akash1r72zrnqd6t7kk6euwfslwccj7pz8s3ez4lmmt5" --dseq 620 --gseq 1 --oseq 1 --provider "akash1czw297jwpk8dymjqzzwduqchwwvh2k3l6zggn3"
```

__t1__: See status of accounts created, market orders, providers, and deployments.
```sh
$ make query-status
```

## Transfer Tokens

__t1__: Send tokens to the _main_ account from _other_
Example of transfering tokens from one account to another.
```sh
$ make send-to-main
```
Transfers `117akash` to the `main` account from `other` account.

__t1__: Query Transaction status
Note the `txhash` from the output of `make send-to-main` command above; use it to set the `TXN=<txhash>` value.
```sh
$ TXN=37E016553DFC9305E8B56D5A8A9EA7E00B9CD6BF860055F7EDC1A2CE3ABCC6EE make query-txn
```
Displays information and status of the transaction. Useful for debugging why a transaction failed to execute.

__t1__: Query _master_ account
```sh
$ NAME=main make query-account
```

## Marketplace

__t3__: Query the market
```sh
$ make query-status
```

__t4__: Create provider
```sh
$ make provider-create
```

__t1__: Query the created provider
```sh
$ make provider-get
```

__t1__: Create Deployment
```sh
$ make deploy
```
Then view the order created from the deployment via `make query-market` or in __t3__. There will be a new Order created.

Bid on the new Order by referencing its sequence IDs:
```sh
../../akashctl --home cache/client query market order list                                    
  {
    "id": {
      "owner": "akash169zth98pgdq8ju0whzvg7zevzaahcvkutsv2sf",
      "dseq": "2010",
      "gseq": 1,
      "oseq": 1,
...
```

Use the `dseq`, `gseq`, and `oseq` variables to configure the `make bid` command. eg:

`DSEQ=2010 GSEQ=1 OSEQ=1 PRICE=2akash make bid` will place a Bid on the Order. This will inform the running Provider to create a Lease if won.
```
../../akashctl --home cache/client  query market lease list                                    
[
  {
    "id": {
      "owner": "akash169zth98pgdq8ju0whzvg7zevzaahcvkutsv2sf",
      "dseq": "2010",
      "gseq": 1,
      "oseq": 1,
      "provider": "akash1caq7dslu2uc3gjg5ys3ulrc8u56k8f6jfm30m3"
    },
    "state": 1,
    "price": {
      "denom": "akash",
      "amount": "2"
    }
  }
]
```

TODO: Provider picking up and running Lease...

__t1__: Ping workload
TODO: Update
```sh
$ make ping
```
