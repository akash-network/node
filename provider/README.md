# Akash Provider Daemon

This folder contains the Akash Provider Daemon. This piece of software listens to events emitted from the Akash blockchain (code in `../app/app.go`) and takes actions on a connected Kubernetes cluster to provision compute capacity based on the bids that the configured provider key wins. The following are the pieces of the daemon:

### [`bidengine`](./bidengine)

The bid engine queries for any existing orders on chain, and based on the on-chain provider configuration, places bids on behalf of the configured provider based on configured selling prices for resources. The daemon listens for changes in the configuration so users can use automation tooling to dynamically change the prices they are charging w/o restarting the daemon. You can see the key management code for `provider` tx signing in `cmd/run.go`.

### [`cluster`](./cluster)

The cluster package contains the necessary code for interacting with clusters of compute that a `provider` is offering on the open marketplace to deploy orders on behalf of users creating `deployments` based on `manifest`s. Right now only `kubernetes` is supported as a backend, but `providers` could easily implement other cluster management solutions such as OpenStack, VMWare, OpenShift, etc...

### [`cmd`](./cmd)

The `cobra` command line utility that wraps the rest of the code here and is buildable.

### [`event`](./event)

Declares the pubsub events that the `provider` needs to take action on won leases an recieved manifests.

### [`gateway`](./gateway)

Contains hanlder code for the rest server exposed by the `provider`

### [`manifest`](./manifest)