### akash Command

`akash` is the command-line interface to the Akash network.  It is used to create/manage/delete Akash deployments.

#### akash validate

Validate a akash configuration file (defaults to `akash.yml`)

```sh
akash validate [ -f akash.yml ]
```

#### akash register

Create a new akash account.

```sh
akash register <email-address> [ -h registry-host ]
```

#### akash create

Create a new deployment for the given deployment.  This will create a [Metacenter](#metacenter) to manage the deployment.

```sh
akash create [ -f akash.yml ] [ -h registry-host ]
```

#### akash list

List all [Metacenters](#metacenter) accessible to this account.

```sh
akash list [ -h registry-host ]
```

#### akash search

List available datacenters that match the [deployment](#deployment) configuration.

```sh
akash search [ -f akash.yml ] [ -h registry-host ]
```

#### akash update

Update a deployment for a akash configuration file (defaults to `akash.yml`)

```sh
akash update [ -f akash.yml ] [ -h metacenter-host ]
```

#### akash delete

Delete a deployment for a akash configuration file (defaults to `akash.yml`)

```sh
akash delete [ -f akash.yml ] [ -h metacenter-host ]
```

#### akash status

Get deployment status for a akash configuration file (defaults to `akash.yml`)

```sh
akash status [ -f akash.yml ] [ -h metacenter-host ]
```

#### akash notify

Delete a deployment for a akash configuration file (defaults to `akash.yml`)

```sh
akash notify [ notification-name ] [ --event <event-name> ] [ -f akash.yml ]
```
