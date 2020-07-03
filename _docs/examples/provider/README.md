# Example: Run Provider in a Kubernetes Environment

* [Prerequisites](#prerequisites)
* [Deploy Provider Services](#deploy-provider-services)
* [Deploy Demo Application](#deploy-demo-application)

## Prerequisites

### Tools

Installed commands:

* [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl)
* [`jq`](https://stedolan.github.io/jq/)
* [`curl`](https://curl.haxx.se/)

### Working Directory

Create staging directory and cd into it

```sh
mkdir akash-demo && cd akash-demo
```

### Kubernetes Cluster

* A Kubernetes cluster with `kubectl` connected to it.
* A route-able IP address for ingress on the cluster.
* A "star" domain pointing to the cluster route-able IP.

See [here](./kube-gce.md) for seting up these prerequisites
using [GCE](https://cloud.google.com/compute).

In this tutorial, we will be using the domain `akashian.io`.

```sh
PROVIDER_DOMAIN=akashian.io
```

### Akash Network

We'll be connecting to the testnet.

```sh
AKASHCTL_NODE=tcp://sentry-1.v2.akashtest.net:26657
AKASHCTL_CHAIN_ID=testnet-v2
```

## Deploy Provider Services

### Download akashctl

```sh
curl -sSfL https://raw.githubusercontent.com/ovrclk/akash/master/godownloader.sh | sh
```

### Configure Akash Client

```sh
export AKASHCTL_HOME="$PWD/home"
mkdir -p "$AKASHCTL_HOME"

./bin/akashctl config node            "$AKASHCTL_NODE"
./bin/akashctl config chain-id        "$AKASHCTL_CHAIN_ID"
./bin/akashctl config keyring-backend test
./bin/akashctl config broadcast-mode  block
./bin/akashctl config trust-node      true
./bin/akashctl config indent          true
./bin/akashctl config output          json
```

### Create Akash Provider Key

```sh
./bin/akashctl keys add provider
```

### Download Kustomize configuration

```sh
curl -s -o kustomization.yaml \
  https://raw.githubusercontent.com/ovrclk/akash/master/_docs/examples/provider/kustomization.yaml
```

### Configure Akash Provider

```sh
cat <<EOF > provider.yaml
host: http://provider.$PROVIDER_DOMAIN
attributes:
  - key: region
    value: us-west-demo-$(whoami)
EOF
```

### Fund your provider account

View your address with

```sh
./bin/akashctl keys show provider -a
```

You can fund the address at the testnet [faucet](https://akash.vitwit.com/faucet).

Ensure you have funds with:

```sh
./bin/akashctl query account "$(./bin/akashctl keys show provider -a)"
```

### Create Akash Provider

Register your provider on the Akash Network:

```sh
./bin/akashctl tx provider create provider.yaml --from provider
```

### Configure provider services client

```sh
cat <<EOF > client-config.txt
node=$AKASHCTL_NODE
chain-id=$AKASHCTL_CHAIN_ID
EOF
```

### Configure provider services

```sh
cat <<EOF > provider-config.txt
ingress-static-hosts=true
ingress-domain=$PROVIDER_DOMAIN
EOF
```

### Configure Provider gateway endpoint

This will configure the kubernetes ingress
to point to your provider.

```sh
cat <<EOF > gateway-host.yaml
- op: replace
  path: /spec/rules/0/host
  value: provider.$PROVIDER_DOMAIN
EOF
```

### Export keys

```sh
echo "password" > key-pass.txt
(cat key-pass.txt; cat key-pass.txt) | ./bin/akashctl keys export provider 2> key.txt
```

### View kubernetes configuration

```sh
kubectl kustomize .
```

### Configure Akash Kubernetes CRDs

```sh
kubectl apply -f https://raw.githubusercontent.com/ovrclk/akash/master/pkg/apis/akash.network/v1/crd.yaml
```

### Install kubernetes configuration

```sh
kubectl kustomize . | kubectl apply -f-
```

### Check status of provider

```sh
./bin/akashctl provider status --provider "$(./bin/akashctl keys show provider -a)"
```

## Deploy Demo Application

### Create key for deployment

```sh
./bin/akashctl keys add deploy
```

### Fund your deployment account

View your address with

```sh
./bin/akashctl keys show deploy -a
```

You can fund the address at the testnet [faucet](https://akash.vitwit.com/faucet).

Ensure you have funds with:

```sh
./bin/akashctl query account "$(./bin/akashctl keys show deploy -a)"
```

### Download Akash SDL file

```sh
curl -s -o deployment.yaml \
  https://raw.githubusercontent.com/ovrclk/akash/master/_docs/examples/provider/deployment.yaml
```

### Customize SDL

```sh
sed -i.bak "s/us-west/us-west-demo-$(whoami)/g" deployment.yaml
```

### Create Deployment

```sh
./bin/akashctl tx deployment create deployment.yaml --from deploy
```

### View Order

```sh
./bin/akashctl query market order list --owner "$(./bin/akashctl keys show deploy -a)"
```

### View Bids

```sh
./bin/akashctl query market bid list --owner "$(./bin/akashctl keys show deploy -a)"
```

### View Leases

```sh
./bin/akashctl query market lease list --owner "$(./bin/akashctl keys show deploy -a)"
```

### Capture Deployment Sequence

__note:__ This can be set when creating the deployment.  It defaults to the
block height at that time.

```sh
DSEQ="$(./bin/akashctl query market lease list    \
  --owner "$(./bin/akashctl keys show deploy -a)" \
  | jq -r '.[0].id.dseq')"
```

### Send Manifest

```sh
./bin/akashctl provider send-manifest deployment.yaml \
  --dseq "$DSEQ" \
  --oseq 1 \
  --gseq 1 \
  --owner    "$(./bin/akashctl keys show deploy -a)" \
  --provider "$(./bin/akashctl keys show provider -a)"
```

### View Lease Status

```sh
./bin/akashctl provider lease-status \
  --dseq "$DSEQ" \
  --oseq 1 \
  --gseq 1 \
  --owner    "$(./bin/akashctl keys show deploy -a)" \
  --provider "$(./bin/akashctl keys show provider -a)"
```

### View Site

```sh
./bin/akashctl provider lease-status \
  --dseq "$DSEQ" \
  --oseq 1 \
  --gseq 1 \
  --owner    "$(./bin/akashctl keys show deploy -a)"     \
  --provider "$(./bin/akashctl keys show provider -a)" | \
  jq -r '.services[0].uris[0]' | \
  while read -r line; do 
    open "http://$line" 
  done
```

### Delete Deployment

```sh
./bin/akashctl tx deployment close --dseq $DSEQ --from deploy
```


