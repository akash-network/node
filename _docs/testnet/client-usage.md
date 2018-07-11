# Introduction
This document describes usage of the Akash client for requesting and managing deployments to the Akash Network.

# Overview of Akash
The Akash Network is a decentralized protocol for provisioning, scaling, and securing cloud workloads. Using Akash, companies (Providers) make their spare server capacity available for containerized deployments by any developer (Tenants). Tenants benefit from access to a a low-cost, decentralized, geographically distributed cloud infrastructure platform whose conventions are very similar to any other cloud provider. Providers benefit by monetizing the idle compute capacity in their on-prem and colocated datacenters.


The Network contains two major functional elements:
 - **Marketplace**: A blockchain-based system that allocates capacity on provider servers to tenants wishing to deploy to them, and transfers payments from tenant to provider in the native Akash (AKSH) token.
 - **Deployment**: A Kubernetes-based system that provisions pods on provider servers and deploys/orchestrates Tenant containers within them.

# Installation
Installation instructions for the client binary may be found [here](https://github.com/ovrclk/akash#installing). Each of these package managers will install both `akashd` (the server) and `akash` (the client). This document describes client usage only.

# The Akash testnet
The Akash testnet is available for public use.  A description of the testnet, registration instructions, and a getting-started guide may be found [here](https://github.com/ovrclk/akash/tree/master/_docs/testnet).

# Top-level commands
These commands are presented as an overview of the features available via the Akash client. Individual command usage is described in subsequent sections.

## Available commands

| Command | Description |
|:--|:--|
| [deployment](#deployment) | Manage deployments. |
| [key](#key) | Manage keys. |
| [logs](#logs) | Service logs |
| [marketplace](#marketplace) | Monitor marketplace. |
| [provider](#provider) | Manage provider. |
| [query](#query) | Query things that need querying. |
| [send](#send) | Send tokens to an account. |
| [status](#status) | Get remote node status. |
| [version](#version) | Print Akash version. |

**Flags**

Every command accepts the following flags. For brevity, they are omitted from the following documentation.

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -h | --help | None | N | Help for any command. |
| -d | --data | String | N | User data directory (defaults to `~/.akash`).  |


# Individual commands

## deployment
Create, manage, and query your deployments.

### Usage

`akash deployment [command]`

### Available commands

| Command | Description |
|:--|:--|
| close | Close a deployment. |
| create | Create a deployment. |
| sendmani | Send manifest to all deployment providers. |
| status | Get deployment status. |
| validate | Validate deployment file. |

### Command usage

#### `close`
Close one of your deployments. Deletes the pod(s) containing your container(s) and stops token transfer.


**Usage**

`akash deployment close <deployment-id> [flags]`

**Example**

```
$ akash deployment close 3be771d6ce0a9e0b5b8caa35d674cdd790f94500226433ab2794ee46d8886f42 -k my-key-name
Closing deployment
```

**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| deployment-id | string | Y | ID of the deployment to close, returned by (`akash query deployment`) |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -k | --key | string | Y | Name of one of your keys, for authentication. Tokens will be transferred from the account associated with this key. |
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |
|  | --nonce | uint | N | Nonce. |


#### `create`
Create a new deployment. Posts your requested to the chain for bidding and subsequent lease creation. Your manifest(s) are then sent to the winning provider(s), pod(s) created, and token transfer from your account to provider(s) initiated.


**Usage**

`akash deployment create <deployment-file> [flags]`

**Example**

```
$ akash deployment create testnet-deployment.yml -k my-key-name
619d25a730f8451b1ba3bf9c1bfabcb469068ad7d8da9a0d4b9bcd1080fb2450
Waiting...
Group 1/1 Fulfillment: 619d25a730f8451b1ba3bf9c1bfabcb469068ad7d8da9a0d4b9bcd1080fb2450/1/2/5ed78fbc526270c3501d09f88a3c442cf1bc6c869eb2d4d6c4f4eb4d41ee3f44 [price=57]
Group 1/1 Fulfillment: 619d25a730f8451b1ba3bf9c1bfabcb469068ad7d8da9a0d4b9bcd1080fb2450/1/2/d56f1a59caabe9facd684ae7f1c887a2f0d0b136c9c096877188221e350e4737 [price=70]
Group 1/1 Lease: 619d25a730f8451b1ba3bf9c1bfabcb469068ad7d8da9a0d4b9bcd1080fb2450/1/2/5ed78fbc526270c3501d09f88a3c442cf1bc6c869eb2d4d6c4f4eb4d41ee3f44 [price=57]
Sending manifest to http://provider.ewr.salusa.akashtest.net...
Service URIs for provider: 5ed78fbc526270c3501d09f88a3c442cf1bc6c869eb2d4d6c4f4eb4d41ee3f44
	webapp: webapp.48bc1ea9-c2aa-4de3-bbfb-5e8d409ae334.147-75-193-181.aksh.io
```
In the example above:
 - **deployment-id**: `619d25a730f8451b1ba3bf9c1bfabcb469068ad7d8da9a0d4b9bcd1080fb2450`
 - **lease**: `619d25a730f8451b1ba3bf9c1bfabcb469068ad7d8da9a0d4b9bcd1080fb2450/1/2/5ed78fbc526270c3501d09f88a3c442cf1bc6c869eb2d4d6c4f4eb4d41ee3f44` (in the form [deployment id]/[deployment group number]/[order number]/[provider address])
 - **service URI**: `webapp.48bc1ea9-c2aa-4de3-bbfb-5e8d409ae334.147-75-193-181.aksh.io`
 - **price**: Given in microAKSH (AKSH * 10^-6).

**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| deployment-file | string | Y | Absolute or relative path to your deployment file. |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -k | --key | string | Y | Name of one of your keys, for authentication. |
|  | --no-wait | none | N | Exit before waiting for lease creation. |
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |
|  | --nonce | uint | N | Nonce |

#### `sendmani`
Sends manifest directly to a deployment's provider(s), using data from the deployment file. Use this command after creating a deployment using the `--no-wait` flag.

**Usage**

`akash deployment sendmani <deployment-file> <deployment-id> [flags]` 

**Example**

```
$ akash deployment sendmani testnet-deployment.yml 619d25a730f8451b1ba3bf9c1bfabcb469068ad7d8da9a0d4b9bcd1080fb2450 -k my-key-name
$
```

**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| manifest | String | Y | **?** |
| deployment-id | string | Y | ID of the deployment to for which to send the manifest, returned by (`akash query deployment`.  |


**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -k | --key | string | Y | Name of one of your keys, for authentication. |
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |

#### `status`
Get the lease and service URI(s) for an open deployment.

**Usage**

`akash deployment status <deployment-id> [flags]`

**Example**

Deployment is open
```
$ akash deployment status 619d25a730f8451b1ba3bf9c1bfabcb469068ad7d8da9a0d4b9bcd1080fb2450
lease: 619d25a730f8451b1ba3bf9c1bfabcb469068ad7d8da9a0d4b9bcd1080fb2450/1/2/5ed78fbc526270c3501d09f88a3c442cf1bc6c869eb2d4d6c4f4eb4d41ee3f44
	webapp: webapp.9060b8ae-1b62-47ff-a247-164f2f081681.147-75-193-181.aksh.io
```

Deployment is closed
```
$ akash deployment close 3be771d6ce0a9e0b5b8caa35d674cdd790f94500226433ab2794ee46d8886f42 -k my-key-name
$
```

**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| deployment-id | string | Y | ID of the deployment to check, returned by `akash query deployment`  |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |

#### `validate`
Validate the syntax and structure of a deployment file.

**Usage**

`akash deployment validate <deployment-file> [flags]`

**Example**

File passes validation
```
$ akash deployment validate testnet-deployment.yml 
ok
```

File does not pass validation (min price too low)
```
$ akash deployment validate badfile.yml
Error: group specs: group san-jose: price too low (1 >= 25 fails)
```

**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| deployment-file | string | Y | Absolute or relative path to your deployment file. |

**Flags**

None


## key
Create and manage your keys.

### Usage

`akash key [command]`

### Available commands

| Command | Description |
|:--|:--|
| create | Create new key |
| list | List all your keys |
| show | Display a single key |

### Command usage

#### `create`
Create a new key to use in the Akash Network. A key links to an Akash account and is used to authenticate to the network.

**Usage**

`akash key create <key-name> [flags]`

**Example**

```
$ akash key create my-key-name
8d2cb35f05ec35666bbc841331718e31415926a1
```
In the example above:
 - **key value**: `8d2cb35f05ec35666bbc841331718e31415926a1`


**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| key-name | string | Y | A meaningful-to-you name for your key. |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -t | --type | (secp256k1\|ed25519\|ledger) | N | Type of key (default "ed25519"). |

#### `List`
List all your local keys.

**Usage**

`akash key list [flags]`

**Example**

```
$ akash key list
my-key-name 		8d2cb35f05ec35666bbc841331718e31415926a1
my-other-key-name 	35c055f1fa38cb1864e484a1d733a58bbffb1156
```

**Arguments**

None

**Flags**

None


#### `show`
Show the key value belonging to a key name.

**Usage**

`akash key show <key-name> [flags]`

**Example**

```
$ akash key show my-key-name
8d2cb35f05ec35666bbc841331718e31415926a1
```

**Arguments**

None

**Flags**

None

## logs
Tail the application logs for each of your services.

**Usage**

`akash logs <service> <lease> [flags]`


**Example**

```
$ akash logs webapp 619d25a730f8451b1ba3bf9c1bfabcb469068ad7d8da9a0d4b9bcd1080fb2450/1/2/5ed78fbc526270c3501d09f88a3c442cf1bc6c869eb2d4d6c4f4eb4d41ee3f44 -f
[webapp-64bcb5d547-fblkv] 2018-08-01T00:08:51.307976982Z 192.168.0.1 - - [01/Aug/2018:00:08:51 +0000] "GET / HTTP/1.1" 200 3583 "-" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/67.0.3396.87 Safari/537.36" "73.162.194.173"
[webapp-64bcb5d547-fblkv] 2018-08-01T00:08:51.614215684Z 192.168.0.1 - - [01/Aug/2018:00:08:51 +0000] "GET /css/main.css HTTP/1.1" 200 195072 "http://webapp.9060b8ae-1b62-47ff-a247-164f2f081681.147-75-193-181.aksh.io/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/67.0.3396.87 Safari/537.36" "73.162.194.173"
[webapp-64bcb5d547-fblkv] 2018-08-01T00:08:51.712794998Z 192.168.0.1 - - [01/Aug/2018:00:08:51 +0000] "GET /images/qr.png HTTP/1.1" 200 7039 "http://webapp.9060b8ae-1b62-47ff-a247-164f2f081681.147-75-193-181.aksh.io/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/67.0.3396.87 Safari/537.36" "73.162.194.173"
```
In the example above, `webapp` is a simple web page serving static content.


**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| service | string | Y | The service name originally defined in your deployment file |
| lease | string | Y | The lease ID belonging to that service, returned by `akash deployment status` |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -f | --follow | none | N | Whether update the console with new log lines or simply return the last n lines defined by `-l`. |
| -l | --lines | uint | N | Number of lines from the end of the logs to show per service (default 10). |
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |

## marketplace
Monitor marketplace transactions.

**Usage**

`akash marketplace [flags]`


**Example**

```
$ akash marketplace
DEPLOYMENT CREATED	4b24d14fe47d1b360fb6cebd883a5ba65f9876e62ba1ac27ace79001b42475e8 created by 35c055f1fa38cb1864920e2a7619d4f95d18c125
ORDER CREATED	4b24d14fe47d1b360fb6cebd883a5ba65f9876e62ba1ac27ace79001b42475e8/1/2
FULFILLMENT CREATED	4b24d14fe47d1b360fb6cebd883a5ba65f9876e62ba1ac27ace79001b42475e8/1/2 by 5ed78fbc526270c3501d09f88a3c442cf1bc6c869eb2d4d6c4f4eb4d41ee3f44 [price=48]
FULFILLMENT CREATED	4b24d14fe47d1b360fb6cebd883a5ba65f9876e62ba1ac27ace79001b42475e8/1/2 by d56f1a59caabe9facd684ae7f1c887a2f0d0b136c9c096877188221e350e4737 [price=54]
LEASE CREATED	4b24d14fe47d1b360fb6cebd883a5ba65f9876e62ba1ac27ace79001b42475e8/1/2 by 5ed78fbc526270c3501d09f88a3c442cf1bc6c869eb2d4d6c4f4eb4d41ee3f44 [price=48]
```
In the example above, `price` is given in microAKSH (AKSH * 10^-6).

**Arguments**

None

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |

## provider
Manage providers. This command is intended for providers, so this section will only address commands that are useful for a client creating and managing deployments.

### Usage

`akash provider [command]`

### Available commands

| Command | Description |
|:--|:--|
| closef | Close an open fulfillment. Used by providers; not documented here.|
| closel | Close an active lease. Used by providers; not documented here. |
| create | Create a provider. Used by providers; not documented here. |
| run | Respond to chain events. Used by providers; not documented here. |
| status | Print provider details. |

### Command usage
#### `status`
Retrieve the attributes and status of one or more providers.

**Usage**

`akash provider status [<provider-id> ...] [flags]`

**Example**

```
$ akash provider status d714ecb330d5a3873bdc88e9fce10dab1a65287fac4fe55c80ac48776fa83276
[
 {
  "Provider": {
   "address": "d714ecb330d5a3873bdc88e9fce10dab1a65287fac4fe55c80ac48776fa83276",
   "owner": "59e018689248c527ed8a755a9c67ec647ce77d28",
   "hostURI": "http://provider.sjc.arrakis.akashtest.net",
   "attributes": [
    {
     "name": "region",
     "value": "sjc"
    }
   ]
  },
  "Status": {
   "code": 200,
   "version": {
    "version": "0.3.3",
    "commit": "4786994cf709e2829aadf64d05b07212e4a8ce28",
    "date": "2018-07-31T20:43:05Z"
   },
   "message": "OK"
  }
 }
```

**Arguments**

None

**Flags**

None

## query
Query all the things that need querying.

### Usage

`akash query [command]`

### Available commands

| Command | Description |
|:--|:--|
| account | Query account details. |
| deployment | Query deployment details. |
| deployment-group | Query deployment-group details. |
| fulfillment | Query fulfillment details. |
| lease | Query lease details. |
| order | Query order details. |
| provider | Query provider details. |

### Command usage

#### `account`
Retrieve the details for one or more of your accounts, including token balance.

**Usage**

`akash query account [account ...] [flags]`

**Example**

```
$ maisy:~ nalesandro$ akash query account -k my-key-name
{
  "address": "8d2cb35f05ec35666bbc841331718e31415926a1",
  "balance": 90351025,
  "nonce": 7
}
```
In the example above, token balance is given in microAKSH (AKSH * 10^-6).


**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| account | string | N | One or more account addresses to query. Omitting this argument returns all your accounts for the provided key. |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -k | --key | string | Y | Name of one of your keys, for authentication. |
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |

#### `deployment`
Retrieve the details for one or more of your deployments. A deployment represents a request for provider resources.

**Usage**

`akash query deployment [deployment ...] [flags]`

**Example**

```
$ akash query deployment -k alpha
{
  "items": [
    {
      "address": "3be771d6ce0a9e0b5b8caa35d674cdd790f94500226433ab2794ee46d8886f42",
      "tenant": "8d2cb35f05ec35666bbc841331718e31415926a1",
      "state": 2,
      "version": "8e02ba39187cbd2de194a7ac3b31ffe9889856d4b817fc039669e569fde6c647"
    },
    {
      "address": "4b24d14fe47d1b360fb6cebd883a5ba65f9876e62ba1ac27ace79001b42475e8",
      "tenant": "8d2cb35f05ec35666bbc841331718e31415926a1",
      "version": "8e02ba39187cbd2de194a7ac3b31ffe9889856d4b817fc039669e569fde6c647"
    },
...
  ]
}
```
In the example above:
 - **"state": 2**: indicates a closed deployment.  
 - **version**: is a hash of the manifest, used by provider to verify incoming manifest content


**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| deployment | string | N | One or more deployment ids to query. Omitting this argument returns all your deployments. |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -k | --key | string | Y | Name of one of your keys, for authentication. |
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |


#### `fulfillment`
Retrieve the details for one or more fulfillments made for your deployments. A fulfillment represents a provider's bid on your deployments. 

**Usage**

`akash query fulfillment [fulfillment ...] [flags]`

**Example**

```
$ akash query fulfillment
{
  "items": [
    {
      "id": {
        "deployment": "2a15e3d0a5ed9201f46f9d4c8e0a80579d202b6bee90ff7fac613f1b289bdf9d",
        "group": 1,
        "order": 2,
        "provider": "4be226880fce4efd19f81c87cebc86bf001e05a7aae7b862d421f3ec36f9e345"
      },
      "price": 71
    },
    {
      "id": {
        "deployment": "3be771d6ce0a9e0b5b8caa35d674cdd790f94500226433ab2794ee46d8886f42",
        "group": 1,
        "order": 2,
        "provider": "5ed78fbc526270c3501d09f88a3c442cf1bc6c869eb2d4d6c4f4eb4d41ee3f44"
      },
      "price": 73,
      "state": 2
    },
...
  ]
}
```
In the example above, `"state": 2` indicates a closed fulfillment.

**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| fulfillment | string | N | One or more fulfillment ids to query. Omitting this argument returns all fulfillments that resulted in leases. |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |


#### `lease`
Retrieve the details for one or more of your leases. A lease represents an agreement between you and the lowest-bidding provider to provide resources as for the price specified in their  fullfillment.

**Usage**

`akash query lease [lease ...] [flags]`

**Example**

```
$ akash query lease -k my-key-name
{
  "items": [
    {
      "id": {
        "deployment": "3be771d6ce0a9e0b5b8caa35d674cdd790f94500226433ab2794ee46d8886f42",
        "group": 1,
        "order": 2,
        "provider": "d56f1a59caabe9facd684ae7f1c887a2f0d0b136c9c096877188221e350e4737"
      },
      "price": 52,
      "state": 2
    },
    {
      "id": {
        "deployment": "4b24d14fe47d1b360fb6cebd883a5ba65f9876e62ba1ac27ace79001b42475e8",
        "group": 1,
        "order": 2,
        "provider": "5ed78fbc526270c3501d09f88a3c442cf1bc6c869eb2d4d6c4f4eb4d41ee3f44"
      },
      "price": 48
    },
...
  ]
}
```
In the example above, `"state": 2` indicates a closed lease.

```
$ akash query lease 3be771d6ce0a9e0b5b8caa35d674cdd790f94500226433ab2794ee46d8886f42/1/2/d56f1a59caabe9facd684ae7f1c887a2f0d0b136c9c096877188221e350e4737
{
  "id": {
    "deployment": "3be771d6ce0a9e0b5b8caa35d674cdd790f94500226433ab2794ee46d8886f42",
    "group": 1,
    "order": 2,
    "provider": "d56f1a59caabe9facd684ae7f1c887a2f0d0b136c9c096877188221e350e4737"
  },
  "price": 52,
  "state": 2
}
```
In the example above, the lease is specified in the form [deployment id]/[deployment group number]/[order number]/[provider address] and the `-k` flag is not required.


**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| lease | string | N | One or more leases to query. Omitting this argument returns all your leases. |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -k | --key | string | Conditional | Name of one of your keys, for authentication. Required when fetching all an account's leases, but not when fetching one lease. |
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |


#### `order`
Retrieve the details for one or more of your orders.  An order is an internal representation of a deplyoyment group: the resources from your deployment that may be fulfilled by a single provider.

**Usage**

`akash query order [order ...] [flags]`

**Example**

```
$ $ akash query order
{
  "items": [
    {
      "id": {
        "deployment": "16bfd04ba37ca64ba675e47d2fb5fcab6c5c3c3e949d71f0012cd65a81dd6507",
        "group": 1,
        "seq": 2
      },
      "endAt": 3519,
      "state": 2
    },
    {
      "id": {
        "deployment": "2a15e3d0a5ed9201f46f9d4c8e0a80579d202b6bee90ff7fac613f1b289bdf9d",
        "group": 1,
        "seq": 2
      },
      "endAt": 204,
      "state": 1
    },
...
  ]
}
```
In the example above:
 - **"state": 2**: indicates a closed order. 
 - **endAt**: indicates the block number upon which all fulfillments must be issued, prior to awarding a lease


**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| order | string | N | One or more order ids to query. Omitting this argument returns all your orders. |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |

#### `provider`
Retrieve the attributes of one or more providers.

**Usage**

`akash query provider [provider ...] [flags]`

**Example**

```
$ $ akash query provider
{
  "providers": [
    {
      "address": "0253c080e189825da0e072ed8213947bb5d9386f4504ab9c15a15f5776600e83",
      "owner": "73ff91326664be3dad53b3b58e9d1fe08dfbec74",
      "hostURI": "http://provider.ewr.caladan.akashtest.net",
      "attributes": [
        {
          "name": "region",
          "value": "ewr"
        }
      ]
    },
    {
      "address": "4be226880fce4efd19f81c87cebc86bf001e05a7aae7b862d421f3ec36f9e345",
      "owner": "e6956171534f8ffbcf47c6830788df4ebbb165a9",
      "hostURI": "http://provider.sjc.arrakis.akashtest.net",
      "attributes": [
        {
          "name": "region",
          "value": "sjc"
        }
      ]
    },
...
  ]
}
```

**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| provider | string | N | One or more provider ids to query. Omitting this argument returns all providers in the network. |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |

## send
Send tokens from one account to another.

**Usage**

`akash send <amount> <to-account> [flags]`


**Example**

```
$ akash send 1.1 35c055f1fa38cb1864e484a1d733a58bbffb1156 -k alpha
Sent 1.1 tokens to 35c055f1fa38cb1864e484a1d733a58bbffb1156 in block 61049
```
In the example above, the amount is given in AKSH.  You may also specify the amount in microAKSH (AKSH * 10^-6) using the `u` unit suffix (e.g. `100u` for 100 microAKSH).

**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| amount | float | Y | The amount of tokens to send. |
| to-account | string | Y | The key value for the recipient account.  |


**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -k | --key | string | Y | Name of one of your keys, for authentication. Tokens will be sent from this account. |
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |
|  | --nonce | uint | N | Nonce. |

## status
Get the status of a remote node.

**Usage**

`akash status [flags]`


**Example**

```
$ akash status
Block: 61553
Block Hash: 734FBC125E094CBC18311B85B3D278E820891D06
```

**Arguments**

None


**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
| -n | --node | string | N | Node host (defaults to https://api.akashtest.net:80). |

## version
Get the client version.

**Usage**

`akash version [flags]`


**Example**

```
$ akash version
version:  0.3.4
commit:   8e90774b47cc3791603d443301038e4dbf49c748
date:     2018-08-01T06:45:59Z
```

**Arguments**

None


**Flags**

None
