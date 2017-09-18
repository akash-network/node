# Photon

Photon is a cloud infrasture system that pairs independent datacenter providers with users seeking high-performance application hosting.  The process is simple for both sides of the equation - [Datacenter Components](#datacenter) are easy to install and provide a high degree of automation, while application deployment [configuration](#deployment-configuration) and [administration](#photon-command) is simple and intuitive.

 * [Photon Components](#photon-components)
   * [Registry](#registry)
   * [Datacenter](#datacenter)
   * [Metacenter](#metacenter)
   * [Deployments](#deployment-configuration)
   * [`photon` Command](#photon-command)
 * [Credentials](#credentials)
 * [Building](#building)
   * [Dependencies](#dependencies)
   * [Photon](#photon-1)

## Components

These components make up the public-facing Photon system.

### Registry

[Datacenters](#datacenter) and [clients](#photon) join the network by registering
with the registry. Clients create a [metacenter](#metacenter) to manage each deployment.

[metacenters](#metacenter) and [datacenters](#datacenter) report usage back to the registry on a regular basis.

Some registry functions will be replaced by a public blockchain.

The Registry API is documented [here](api/registry/registry.proto).

### Datacenter

Each datacenter will host an agent which is a mediator between the with the Photon Network ([Metacenter](#metacenter),[Registry](#registry)) and datecenter-local infrastructure.

The agent has admistrator access to the datacenter-local provisioning and management services and will:

 * Advertise availability and pricing to network
 * Create/Delete/Monitor on-premesis resources for clients.
 * Execute market transactions for the resources running in the datacenter.

The Datacenter API is documented [here](api/datacenter/datacenter.proto).

### Metacenter

Metacenter manages a deployment for a user.  A deployment consists of:

 * maintaining desired number of nodes in given set of datacenter profiles
 * cross-datacenter autoscaling
 * deployment modifications: additional services, service versions, desired nodes, etc...

The Metacenter API is documented [here](api/metacenter/metacenter.proto).

#### Metacenter Account

Metacenters are initialized with one account - the creating account is the owner.  Other accounts may be added by the owner, optionally with a short-lived timespan.

### Deployment Configuration

Deployment services, datacenters, pricing, etc.. are described by a [YAML](http://www.yaml.org/start.html) configuration file.  Configuration files may end in `.yml` or `.yaml`.

Deployments can be composed of multiple files via the [include](#include) section.  A complete deployment has the following sections:

* [version](#version)
* [include](#include) (optional)
* [services](#include)
* [deployment](#deployment)
* [notifications](#notifications) (optional)

An example deployment configuration can be found [here](_docs/deployment.yml).

#### version

Indicates version of Photon configuration file.  Currently only `"0.1"` is accepted.

#### include

List of external configuration files to include.  Remote files are accepted via `http(s)://` prefix.

When a configuration file is included, its `include`, `services` and `deployment` structures are deply merged (note: show examples).

All included configurations must have the same `version` field (note: relax this).

Examples:

| Included File | Effect |
| --- | --- |
| `"x.yml"` | Include file named "x.yml" in same directory as current file |
| `"/tmp/x.yml"` | Include file at absolute path `/tmp/x.yml` |
| `"https://test.com/x.yml"` | Include file hosted at `https://test.com/x.yml` |

#### services

The top-level `services` entry contains a map of workloads to be ran on the Photon deployment.  Each key is a service name; values are a map containing the following keys:

| Name | Required | Meaning |
| --- | --- | --- |
| `image` | Yes | Docker image of the container |
| `depends-on` | No | List of services which must be brought up before the current service |
| `args` | No | Arguments to use when executing the container |
| `env` |  No | Environment variables to set in running container |
| `expose` | No | Entities allowed to connec to to the services.  See [services.expose](#servicesexpose). |

##### services.expose

`expose` is a list describing what can connect to the service.  Each entry is a map containing one or more of the following fields:

| Name | Required | Meaning |
|--- | --- | --- |
| `port` | Yes | Port to expose |
| `proto` | No | Protocol type (`tcp`,`http`, or `https`) |
| `to` | No | List of entities allowed to connect.  See [services.expose.to](#servicesexposeto) |

The `port` value governs the default `proto` value as follows:

| `port` | `proto` default |
| --- | --- |
| 80 | http |
| 443 | https |
| all others | tcp |

##### services.expose.to

`expose.to` is a list of clients to accept connections from.  Each item is a map with one or more of the following entries:

| Name | Value | Default | Description |
| --- | --- | --- | --- |
| `service` | A service in this deployment | | Allow the given service to connect |
| `global`  | `true` or `false` | `false` | If true, allow connections from outside of the datacenter |


If no service is given and `global` is true, any client can connect from anywhere (web servers typically want this).

If a service name is given and `global` is `false`, only the services in the current datacenter can connect.
If a service name is given and `global` is `true`, services in other datacenters for this deployment can connect.

If `global` is `false` then a service name must be given.

#### deployment

The `deployment` section defines how to deploy the defined services.  It allows for defining required resources and pricing for an arbitrary number of datacenters.

It contains two fields: `datacenters` for defining profiles of desired compute resources, and `services` for defining how to deploy services to profiles.

`deployment.datacenters` and `deployment.services` are both used to find datacenters to deploy to.

##### deployment.datacenters

`datacenters` is a map of desired datacenter attributes.  Each entry will match one datacenter in the marketplace.  Entry keys are the name of the datacenter and will be referenced later in the deployment section.

Each entry has two keys: `region` and `profiles`.  `region` is one of a standard set of regions that photon recognizes.  `profiles` configures the desired resource profiles.  For more on profiles, see [deployment.datacenters.profiles](#deploymentdatacentersprofiles).

Example:

```yaml
datacenters:
  westcoast:
    region: us-west
    profiles:
      web:
        qos:
          lifetime: medium
          memory: 3GB
        pricing:
```

This creates a datacenter called `westcoast` with in the `us-west` region with one profile called `web`.

###### deployment.datacenters.profiles

Each entry has a name (`web` in the example above), a Quality Of Service (`qos`) section, and a section for pricing (`pricing`).  The profile will be referenced by name in the [deployment.services](#deploymentservices) section when defining how to deploy services.

##### deployment.services

`services` is a mapping of a [service](#services) to a [datacenter profile](#deploymentdatacenters).  Each key is the service name and the value describes
the datacenter profiles to deploy that service to.

Example:

```yaml
services:
  web-tier:
    westcoast:
      profile: web
      nodes: 20
```

This says that the `web-tier` service should deploy to 20 nodes in the `web` profile of the choosen `westcoast` datacenter.

#### notifications

`notifications` is a list of recepients to send notification events to.  Each entry contains a `name`, `type`, and an optional `events` field.

 * `name`: arbitrary name of notification.
 * `type`: notification type.  available options: `email`, `webhook`.
 * `events`: list of events to send notification on.  if no `events` field is given, all events trigger a notification.

Event list:

|Event Name| Description|
|---|---|
|cluster.created|A new cluster has been created|
|cluster.down|An previously-existing cluster is no longer available|
|cluster.node.created|A new node has been created in a cluster|
|cluster.node.down|A new node has been created in a cluster|
|deploy.begin|A deployment has begun being created or updated|
|deploy.complete|A deployment has completed being created or updated|
|deploy.failure|A deployment has encountered an error during creation or while updating|

### photon Command

`photon` is the command-line interface to the Photon network.  It is used to create/manage/delete Photon deployments.

#### photon validate

Validate a photon configuration file (defaults to `photon.yml`)

```sh
photon validate [ -f photon.yml ]
```

#### photon register

Create a new photon account.

```sh
photon register <email-address> [ -h registry-host ]
```

#### photon create

Create a new deployment for the given deployment.  This will create a [Metacenter](#metacenter) to manage the deployment.

```sh
photon create [ -f photon.yml ] [ -h registry-host ]
```

#### photon list

List all [Metacenters](#metacenter) accessible to this account.

```sh
photon list [ -h registry-host ]
```

#### photon search

List available datacenters that match the [deployment](#deployment) configuration.

```sh
photon search [ -f photon.yml ] [ -h registry-host ]
```

#### photon update

Update a deployment for a photon configuration file (defaults to `photon.yml`)

```sh
photon update [ -f photon.yml ] [ -h metacenter-host ]
```

#### photon delete

Delete a deployment for a photon configuration file (defaults to `photon.yml`)

```sh
photon delete [ -f photon.yml ] [ -h metacenter-host ]
```

#### photon status

Get deployment status for a photon configuration file (defaults to `photon.yml`)

```sh
photon status [ -f photon.yml ] [ -h metacenter-host ]
```

#### photon notify

Delete a deployment for a photon configuration file (defaults to `photon.yml`)

```sh
photon notify [ notification-name ] [ --event <event-name> ] [ -f photon.yml ]
```

## Credentials

All communications are encrypted and authenticated via mTLS.  The [CFSSL](https://github.com/cloudflare/cfssl) library and CA server will be used for generating and managing certificates.

The [Registry](#registry) owns the master self-signed root certificate and issues both leaf certificates and intermediate certificates as necessary.

 * Each [Registry](#registry) holds a root self-signed cert.
 * Each Account receives a leaf certificate and CA bundle.
 * Each [Metacenter](#metacenter) receives a leaf certificate and a CA bundle.
 * Each [Datacenter](#datacenter) receives an intermediate certificate and a CA bundle.
 * Each [Metacenter Account](#metacenter-account) has a unique leaf certificate created by the [Metacenter](#metacenter) to be used for connecting to the [Metacenter](#metacenter)

### Account Creation

Client generates public,private keypair and sends the public key to the [Registry](#registry).  The [Registry](#registry) responds with
a certificate and CA bundle.

### Metacenter Creation

Client generates a public,private keypair to use for the [Metacenter Account](#metacenter-account).  A [Metacenter](#metacenter) is launched with an intermediate Certificate.  A metacenter-signed certificate based off the client's sent public key and a CA bundle for connecting to the [Metacenter](#metacenter) is returned to the client.

### Datacenter Creation

Client generates public,private keypair and sends the public key to the [Registry](#registry).  The [Registry](#registry) responds with an intermediate certificate and CA bundle to be used by the new Datacenter.

The client launches the [datacenter](#datacenter) instance with the new intermediate cert and CA bundle.

### Datacenter Deploy

For each service needing connection to or from another datacenter, a datacenter creates a public,private keypair and has the [Metacenter](#metacenter) generate a certificate for it.

This generated cert is used by both sides of a connection for cross-datacenter communications between services.

## Building

### Dependencies

 * [glide](https://github.com/Masterminds/glide):
 * [protocol buffers](https://developers.google.com/protocol-buffers/)
 * [protoc-gen-go](https://github.com/golang/protobuf)

#### MacOS:

```sh
brew install glide
brew install protobuf
go get -u github.com/golang/protobuf/protoc-gen-go
```

#### Arch Linux:

```sh
curl https://glide.sh/get | sh
sudo pacman -Sy protobuf
go get -u github.com/golang/protobuf/protoc-gen-go
```

### Photon

Download and build photon:

```sh
go get -d github.com/ovrclk/photon
cd $GOPATH/src/github.com/ovrclk/photon
make deps-install
make
```
