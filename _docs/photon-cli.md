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
