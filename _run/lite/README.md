Lite example of Akash tooling
-----------------------------

This lightweight example showcases Akash node and CLI tool commands for initializing Keys, Accounts, Genesis file,
running an `akashd` node, querying state of the blockchain data, creating bids, but not able to run provider(yet).

## Runbook

### Initialize

`make init`: Creates accounts and keys for use in the Cosmos/Tendermint blockchain, updates genesis file with accounts and grants some tokens, `akashd` creates a validator transaction for itself, final collection for the Genesis file. 

### Run local Network

`make run-daemon` executes the `akashd` binary and runs a Akash network node setup as a validator!

### Querying

`make query-status` uses `akashctl` to execute several queries against the running node.

**Queries**
* Accounts: `main` and `provider` information using their addresses.
* Lists all Deployments on the network.
* Market orders, bids, and leases are listed.
* Providers available.


### Provider

`make provider`: Creates an instance of the provider which can bid on orders. With a running Provider, Deployments can now be requested to be run.

### Deployment

`make deploy` creates a Tenant's transaction requesting a deployment to be fulfilled by a Provider. This is written into the blockchain for Providers to bid on, and contains a `profile` of requirements which filters the Providers which can bid on it.

`make deploy-close` terminates an active deployment using DSEQ to specify the target.

### Bidding

Providers bid on Deployment requests, and once processed by the Akash network, become `Orders`.

The Makefile contains several bidding commands to simulate Akash network transactions. Each command can be specially configured with the DSEQ, GSEQ, and OSEQ parameters enable targeting unique Orders and Bids.

`make bid` sends a transaction `--from provider` on an Tenants Deployment. 

`make bid-close` terminates a previously created Bid on an Deployment.

`make order-close` terminates a requested Order from `make deploy`.
