# Persistent Storage Setup and usage

## Common

### Storage class

**Storage classes**
At this moment there are three storage classes supported

| Class   | Throughput/Approx matching device        | Latency          |
|---------|------------------------------------------|------------------|
| beta1   | hdd                                      | TBD              |
| beta2   | ssd                                      | TBD              |
| beta3   | NVMe                                     | TBD              |
| default | is alias to any of the supported classes | depends on class |

**Purpose of having `default` class**
In many cases tenants may not want/need to deal with or understand differences between storage classes. Most providers will support just one storage class and may want to ~~alias~~ map it

for example

**provider1** just one storage class which maps to the `default` as well

| Class |         |
|-------|---------|
| beta1 | default |

**provider2** default maps to `beta1`

| Class |         |
|-------|---------|
| beta1 | default |
| beta2 |         |

**provider3** default maps to `beta3`

| Class |         |
|-------|---------|
| beta1 |         |
| beta2 |         |
| beta3 | default |

**provider4** no default class

| Class |     |
|-------|-----|
| beta1 |     |
| beta2 |     |


## Tenant

### SDL

The SDL layout has updates needs to be used in order to utilize persistent storage. All references below are based on the following [example](#SDL-Example)

#### Profiles

##### Multiple storage entries

**NOTE** maximum amount of volumes are limited for 2 per profile!

In `profile` the storage has been upgraded to support multiple entries. Both old and new notations are supported. Both examples below are showing valid use-cases of the sdl

```yaml
storage:
  size: 512Mi
```

```yaml
storage:
  - size: 512Mi
  - name: data
    size: 1Gi
```

##### Storage name (alias)

Each entry has new field `name` used by [services](#services) to reference various storage specific parameters. It can be omitted for single value usecase and default value is set to `default`

**valid**

```yaml
storage:
  size: 512Mi
  # name is set to "default"
---
storage:
  size: 512Mi
  name: data
---
storage:
  - size: 512Mi
    # name is set to "default"
  - name: data
    size: 1Gi
```

**invalid**

```yaml
storage:
  - size: 512Mi
    # name is set to "default"
  - size: 1Gi
    # should have name set
```

##### Storage attributes

1. `persistent` - either volume requires persistence or not. Default value is set to `false`
2. `class` - storage class for persistent volumes. Default value is set to `default`. It is error to set storage class for non-persistent volumes

```yaml
storage:
  - size: 512Mi
    attributes:
      class: beta # error. ephemeral storage should not have storage class set
  - name: data
    size: 1Gi
    attributes:
      persistent: true
      class: beta2
```

#### Services

If deployment references profile with multiple volumes then service **must** include params section to configure mount points.

all examples in this section below have been shortened and have only information required to show usage of storage

##### Example 1 Ephemeral storage only

```yaml
services:
  grafana:
    image: grafana/grafana
profiles:
  compute:
    grafana:
      resources:
        cpu:
          units: 1
        memory:
          size: 1Gi
        storage:
          - size: 512Mi
```

##### Example 2 Mount point to the volume with name

```yaml
services:
  grafana:
    image: grafana/grafana
    params:
      storage:
        data: # <- matches to the name of the volume-|
          mount: /var/lib/grafana #                    |
profiles: #                       #                    |
  compute: #                    |
    grafana: #                    |
      resources: #                    |
        storage: #                    |
          - size: 512Mi # ephemeral storage            |
          - name: data  # <----------------------------|
            size: 1Gi
            attributes:
              persistent: true
              class: beta2
```

##### Example 3 Mount point to the volume with default name

```yaml
services:
  grafana:
    image: grafana/grafana
    params:
      storage:
        default: # <- matches to the name of the volume-|
          mount: /var/lib/grafana #                     |
profiles: #                       #                     |
  compute: #                     |
    grafana: #                     |
      resources: #                     |
        storage: #                     |
          - size: 512Mi # ephemeral storage             |
            name: ephemeral #                           |
          - size: 1Gi   # <-----------------------------| the name of the volume is default
            attributes:
              persistent: true
              class: beta2
```

##### Example 3 Multiple services using same compute profile

```yaml
services:
  postgres:
    image: postgres/postgres
    params:
      storage:
        data: # <- matches to the name of the volume-----|
          mount: /var/lib/postgres #                       |
  grafana: #                       |
    image: grafana/grafana         #                       |
    params: #                       |
      storage: #                       |
        data: # <- matches to the name of the volume-|   |
          mount: /var/lib/grafana #                    |   |
profiles: #                       #                    |   |
  compute: #                    |   |
    grafana: #                    |   |
      resources: #                    |   |
        storage: #                    |   |
          - size: 512Mi # ephemeral storage            |   |
          - name: data  # <----------------------------|<--|
            size: 1Gi
            attributes:
              persistent: true
              class: beta2
```

Worth to point whereas different services may reference same compute profile at the result each service has distinct volumes.

## Provider (WIP)

### Create/update on-chain info

On-chain record of the provider must have storage capabilities to participate in bid process on orders with storage

```yaml
host: https://localhost:8443
jwt-host: https://localhost:8444
attributes:
  - key: region
    value: us-west
  - key: capabilities/storage/1/persistent
    value: true
  - key: capabilities/storage/1/class
    value: default
  - key: capabilities/storage/2/persistent
    value: true
  - key: capabilities/storage/2/class
    value: beta2
```

3. Install [inventory-operator](https://github.com/ovrclk/helm-charts/tree/main/charts/inventory-operator)

## SDL Example

```yaml
version: "2.0"
services:
  grafana:
    image: grafana/grafana
    expose:
      - port: 3000
        as: 80
        to:
          - global: true
        accept:
          - webdistest.localhost
    params:
      storage:
        data:
          mount: /var/lib/grafana
profiles:
  compute:
    grafana:
      resources:
        cpu:
          units: 1
        memory:
          size: 1Gi
        storage:
          - size: 512Mi
          - name: data
            size: 1Gi
            attributes:
              persistent: true
              class: beta2
  placement:
    westcoast:
      attributes:
        region: us-west
      pricing:
        grafana:
          denom: uakt
          amount: 1000
deployment:
  grafana:
    westcoast:
      profile: grafana
      count: 1
```

## CEPH/ROOK

### Teardown

#### Zapping devices

Disks on nodes used by Rook for osds must be reset to a usable (aka blank) state with the [following methods](https://github.com/rook/rook/blob/master/Documentation/ceph-teardown.md#zapping-devices)
