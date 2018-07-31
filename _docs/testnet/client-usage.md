# Introduction
This document describes usage of the Akash client for requesting and managing deployments to the Akash Network.

# Overview of Akash
The Akash Network is a decentralized protocol for provisioning, scaling, and securing cloud workloads. Using Akash, companies (Providers) make their spare server capacity available for containerized deployments by any developer (Tenants). Tenants benefit from access to a a low-cost, decentralized, geographically distributed cloud infrastructure platform whose conventions are very similar to any other cloud provider. Providers benefit by monetizing the idle compute capacity in their on-prem and colocated datacenters.


The Network contains two major functional elements:
 - **Marketplace**: A blockchain-based system that allocates capacity on Provider servers to Tenants wishing to deploy to them, and transfers payments from Tenant to Provider in the native Akash (AKSH) token.
 - **Deployment**: A Kubernetes-based system that provisions pods on Provider servers and deploys/orchestrates Tenant containers within them.

# Installation
Installation instructions for the client binary may be found [here](https://github.com/ovrclk/akash#installing). Each of these package managers will install both `akashd` (the server) and `akash` (the client). This document describes client usage only.

# The Akash testnet
The Akash testnet is available for public use.  A description of the testnet, registration instructions, and a getting-started guide may be found [here](https://github.com/ovrclk/akash/_docs/testnet).
  
# Top-level commands
These commands are presented as an overview of the features available via the Akash client. Individual command usage is described in subsequent sections.

## Available commands

| Command | Description |
|:--|:--|
| deployment | Manage deployments |
| help | Help about any command |
| key | Manage keys |
| logs | Service logs |
| marketplace | **TODO** appropriate for client?  Monitor marketplace. |
| provider | **TODO** appropriate for client?  Manage provider. |
| query | Query something **TODO** better |
| send | Send tokens to an account |
| status | Get remote node status |
| version | Print version |

**Flags**

Every command accepts the following flags. For brevity, they are omitted from the following documentation.

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -h | --help | None |  | Help for any command |
| -d | --data | String | N |Data directory (defaults to `~/.akash`) **TODO** what is this |


# Individual commands

## deployment

### Usage

`akash deployment [command]`

### Available commands

| Command | Description |
|:--|:--|
| close | close a deployment |
| create | create a deployment |
| sendmani | send manifest to all deployment providers **TODO** wut |
| status | get deployment status |
| update | update a deployment (*EXPERIMENTAL*) |
| validate | validate deployment file |

### Command usage

#### `close`
**Usage**

`akash deployment close <deployment-id> [flags]`


**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| deployment-id | uuid | Y | ID of the deployment to close, returned by (`akash query deployment`) |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -k | --key | string | Y | Key name |
| -n | --node | string | N | Node host (defaults to http://api.akashtest.net:80) |
|  | --nonce | uint | N | Nonce |


#### `create`
**Usage**

`akash deployment create <deployment-file> [flags]`


**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| deployment-file | string | Y | Absolute or relative path to your deployment file |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -k | --key | string | Y | Key name |
|  | --no-wait | none | N | Exit before waiting for lease creation |
| -n | --node | string | N | Node host (defaults to http://api.akashtest.net:80) |
|  | --nonce | uint | N | Nonce |



#### `name`
**Usage**

`thing`


**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
|  |  |  |  |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
|  |  |  |  |  |





        
      