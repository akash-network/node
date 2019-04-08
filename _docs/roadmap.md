# Akash Network Roadmap

This the Akash Product Roadmap for 2019. Akash Testnet is currently live, to get started head over to [registration](https://akash.network/testnet/). For previous versions of the Roadmap, please check the edit history.

## Mainnet Launch
#### April 2019
Mainnet launch with spot computing marketplace and developer-friendly deployment tools. This release uses production proven Tendermint 0.31 for BFT consensus.


## Scalability with Interoperability
#### May 2019
This milestone improves scalability and interoperability on the Akash Network by providing a mechanism for tenants to limit the deployment to a specific set of providers (and data centers) with an ability to scale the number of instances (containers) for deployment within a single provider. Interledger support for Interoperability with various ledger based systems and allows for frictionless integration with major cloud providers to increase compute supply and remove barriers for enterprise adoption.


## Trusted Providers with Reputation
#### June 2019
This milestone improves Provider Trust with the introduction of provider accreditation features reduces spam with a staking based reputation system. Akash is a permission-less network where any tenant (with a valid key) can consume computing services from any provider that has sufficient computing power in a peer-to-peer manner disintermediating a broker. This provides a challenge for filtering out bad actors. With this milestone, tenants can delegate the attestation to a trusted third-party (Oracle) with an ability for tenants to selectively deploy to such trusted providers. 


## Private Containers
#### July 2019
This milestone provides private containers. Akash currently supports public docker containers - workload containers must be accessible without authentication or authorization.  Private registries are highly preferred to public containers as the latter can leak information about the inner-workings of a system or organization which would otherwise be kept secret.

## Role-based access control (RBAC)
#### August 2019
Role-based access control (RBAC) is a method of regulating access to a computer or network resources based on the roles of individual users within an enterprise. We propose a RBAC system to drive authorization decisions, allowing admins to dynamically configure policies through the Akash API.

An RBAC system would allow tenants to create workloads that operate on deployments.  For example, remote workloads could be granted rights for scaling the deployment within a certain range or cost, but not be allowed to update the workload containers or cancel the deployment completely.

### x.509 Support (SSL)
#### Q4, 2019
Akash currently does not provide a mechanism to attach x.509 certificates to workloads and defers to an external router (Cloudflare) to provide SSL capability. We propose a system where tenants can attach an x.509 key-pair safely to workloads.

Akash will be adopting an open framework such as SPIFFE, suitable to provide the identity framework suitable for most modern workloads. SPIFFE (Secure Production Identity Framework For Everyone) provides a secure identity, in the form of a specially crafted X.509 certificate, to every workload in a modern production environment. SPIFFE removes the need for application-level authentication and complex network-level ACL configuration.

### Fault-tolerant Manifest Distribution
#### Q4, 2019
Deployment manifests are currently uploaded by the tenant to each provider.  When deployments are fault-tolerant (new leases may be created with new providers), this method of manifest distribution requires that tenants be online and active during the lifetime of their deployment.

An alternative, fault-tolerant approach to manifest distribution is to have providers and the tenant join a peer-to-peer file sharing network, similar to BitTorrent.  This approach would allow the manifest to be accessed so long as one peer (tenant or provider) is still active.

Peer-to-peer manifest distribution can also increase the performance of the operation and open the door for private workload container distribution to many providers.
