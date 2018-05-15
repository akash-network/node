## Deployment Configuration

Deployment services, datacenters, pricing, etc.. are described by a [YAML](http://www.yaml.org/start.html) configuration file.  Configuration files may end in `.yml` or `.yaml`.

Deployments can be composed of multiple files via the [include](#include) section.  A complete deployment has the following sections:

 * [version](#version)
 * [include](#include) (optional)
 * [services](#services)
 * [profiles](#profiles)
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

### profiles

The `profiles` section contains named compute and placement profiles to be used in the [deployment](#deployment).

#### profiles.compute

`profiles.compute` is map of named compute profiles.  Each profile specifies compute resources to be leased for each service instance
uses uses the profile.

Example:

```yaml
web:
  cpu: 2
  memory: 3GB
  disk: 5GB
```

This defines a profile named `web` having resource requirements of 2 vCPUs, 2 gigabytes of memory, and 5 gigabytes of disk space available.

#### profiles.placement

`profiles.placement` is map of named datacenter profiles.  Each profile specifies required datacenter attributes and pricing
configuration for each [compute profile](#profilescompute) that will be used within the datacenter.

Example:

```yaml
westcoast:
  attributes:
    region: us-west
  pricing:
    web: 8
    db: 15
```

This defines a profile named `westcoast` having required attributes `{region="us-west"}`, and with a max price for
the `web` and `db` [compute profiles](#profilescompute) of 8 and 15 tokens an hour, respectively.

### deployment

The `deployment` section defines how to deploy the services.  It is a mapping of service name to deployment configuration.

Each service to be deployed has an entry in the `deployment`.  This entry is maps [datacenter profiles](#profilesplacement) to
[compute profiles](#profilescompute) to create a final desired configuration for the resources required for the service.

Example:

```yaml
web:
  westcoast:
    profile: web
    count: 20
```

This says that the 20 instances of the `web` service should be deployed to a datacenter matching the `westcoast` [datacenter profile](#profilesplacement).  Each instance will have 
the resources defined in the `web` [compute profile](#profilescompute) available to it.
