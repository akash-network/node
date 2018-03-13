## Deployment Configuration

Deployment services, datacenters, pricing, etc.. are described by a [YAML](http://www.yaml.org/start.html) configuration file.  Configuration files may end in `.yml` or `.yaml`.

Deployments can be composed of multiple files via the [include](#include) section.  A complete deployment has the following sections:

 * [version](#version)
 * [include](#include) (optional)
 * [services](#include)
 * [deployment](#deployment)

An example deployment configuration can be found [here](deployment.yml).

### version

Indicates version of Akash configuration file.  Currently only `"0.1"` is accepted.

### include

List of external configuration files to include.  Remote files are accepted via `http(s)://` prefix.

When a configuration file is included, its `include`, `services` and `deployment` structures are deply merged (note: show examples).

All included configurations must have the same `version` field (note: relax this).

Examples:

| Included File | Effect |
| --- | --- |
| `"x.yml"` | Include file named "x.yml" in same directory as current file |
| `"/tmp/x.yml"` | Include file at absolute path `/tmp/x.yml` |
| `"https://test.com/x.yml"` | Include file hosted at `https://test.com/x.yml` |

### services

The top-level `services` entry contains a map of workloads to be ran on the Akash deployment.  Each key is a service name; values are a map containing the following keys:

| Name | Required | Meaning |
| --- | --- | --- |
| `image` | Yes | Docker image of the container |
| `depends-on` | No | List of services which must be brought up before the current service |
| `args` | No | Arguments to use when executing the container |
| `env` |  No | Environment variables to set in running container |
| `expose` | No | Entities allowed to connec to to the services.  See [services.expose](#servicesexpose). |

#### services.expose

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

#### services.expose.to

`expose.to` is a list of clients to accept connections from.  Each item is a map with one or more of the following entries:

| Name | Value | Default | Description |
| --- | --- | --- | --- |
| `service` | A service in this deployment | | Allow the given service to connect |
| `global`  | `true` or `false` | `false` | If true, allow connections from outside of the datacenter |


If no service is given and `global` is true, any client can connect from anywhere (web servers typically want this).

If a service name is given and `global` is `false`, only the services in the current datacenter can connect.
If a service name is given and `global` is `true`, services in other datacenters for this deployment can connect.

If `global` is `false` then a service name must be given.

### deployment

The `deployment` section defines how to deploy the defined services.  It allows for defining required resources and pricing for an arbitrary number of datacenters.

It contains two fields: `datacenters` for defining profiles of desired compute resources, and `services` for defining how to deploy services to profiles.

`deployment.datacenters` and `deployment.services` are both used to find datacenters to deploy to.

#### deployment.datacenters

`datacenters` is a map of desired datacenter attributes.  Each entry will match one datacenter in the marketplace.  Entry keys are the name of the datacenter and will be referenced later in the deployment section.

Each entry has two keys: `region` and `profiles`.  `region` is one of a standard set of regions that akash recognizes.  `profiles` configures the desired resource profiles.  For more on profiles, see [deployment.datacenters.profiles](#deploymentdatacentersprofiles).

Example:

```yaml
datacenters:
  westcoast:
    region: us-west
    profiles:
      web:
        compute:
          cpu: 1
          memory: 5GB
          disk: 50GB
        pricing:
          max-price: 10
          collateral: 1d
```

This creates a datacenter called `westcoast` with in the `us-west` region with one profile called `web`.

##### deployment.datacenters.profiles

Each entry has a name (`web` in the example above), the resources required by this group (`compute`), and a section for pricing (`pricing`).  The profile will be referenced by name in the [deployment.services](#deploymentservices) section when defining how to deploy services.

##### deployment.datacenters.profiles.pricing

The pricing section defines pricing rules for orders generated for this order.  It is currently composed of two fields:

 * `max-price`: maximum price in PTN/hour
 * `collateral`: amount of collateral that a datacenter must post when bidding to fulfill the datacenters.  A `collatoral` value of `1d` (one day) means that the datacenter must post collateral of `24 * max-price`.

#### deployment.services

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
