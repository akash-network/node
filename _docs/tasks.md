## design: overlay network 

Bare-bones overlay network design, documented in `_docs/`.

1. Given a manifest, determine all (service, peer) pairs needing a secure connection.
1. Negotiate a secure connection with a peer for each pair.
1. Teardown connections when the lease is no longer active.

Note: do not handle manifest updates.

 * [ ] Determine necessary connections for given manifest distribution.
 * [ ] Monitor for when a peer connection is no longer needed (peer lease inactive).
 * [ ] Certificate design
   * [ ] Ephemeral leaf certs for each connection.
   * [ ] On-chain root cert for each datacenter.
 * [ ] Network Protocol
   * [ ] Open connection to peer (and vice-versa).
   * [ ] State machine for each connection.
     * [ ] Illustrated with [graphviz](https://graphviz.org).
   * [ ] Messages/service declared in [gRPC](https://grpc.io) file.
 * [ ] Illustrate with [mermaid](https://mermaidjs.github.io/)



## order sequence numbers: high chance of consensus confusion

The facilitator engine cannot modify the state.  At the same time, it needs to emit deployment order transactions with a correct sequence number.

The sequence is currently based on the deployment.  This sequence is also used for deployment groups.  This all leads to the possibility of sequence generation being out of sync and consensus taking forever or impossible.

A quick fix might be to have a sequence for each deployment group.


## Conflicting ingress routes

There is good way to resolve conflicting ingress hostnames, results in odd behaviour
