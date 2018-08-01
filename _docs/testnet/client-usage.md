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
| -h | --help | None |  | Help for any command. |
| -d | --data | String | N | Data directory (defaults to `~/.akash`). **TODO** what is this |


# Individual commands

## deployment

### Usage

`akash deployment [command]`

### Available commands

| Command | Description |
|:--|:--|
| close | Close a deployment |
| create | Create a deployment |
| sendmani | Send manifest to all deployment providers **TODO** wut |
| status | Get deployment status |
| validate | Validate deployment file |

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
| deployment-id | uuid | Y | ID of the deployment to close, returned by (`akash query deployment`) |

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
**TODO** What does this do?

**Usage**

`akash deployment sendmani <manifest> <deployment-id> [flags]` **todo rename to deployment-file?**

**Example**

```
$ akash deployment sendmani testnet-deployment.yml 619d25a730f8451b1ba3bf9c1bfabcb469068ad7d8da9a0d4b9bcd1080fb2450 -k my-key-name
$
```

**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
| manifest | String | Y | **?** |
| deployment-id | UUID | Y | ID of the deployment to for which to send the manifest, returned by (`akash query deployment`.  |


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
| deployment-id | UUID | Y | ID of the deployment to check, returned by `akash query deployment`  |

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
| -t | --type | (ed25519\|secp256k1\|ledger) | N | Type of key (default "ed25519"). |

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






=====================
#### `show`
desc

**Usage**

`xxx`

**Example**

```
$ 
```
In the example above:
 - **xxx**: `xxx`


**Arguments**

| Argument | Type | Required | Description |
|:--|:--|:--|:--|
|  |  |  |  |

**Flags**

| Short | Verbose | Argument | Required | Description |
|:--|:--|:--|:--|:--|
|  |  |  |  |  |



        
      