# Example: Run Provider in a Kubernetes Environment

* [Prerequisites](#prerequisites)
* [Deploy Provider Services](#deploy-provider-services)
* [Deploy Demo Application](#deploy-demo-application)

## Prerequisites

* Akash Network to connect to.
  * Established account address with some tokens(for gas) in network to create Provider registration.
* Kubernetes Cluster
  * [Akash Kubespray](https://github.com/ovrclk/kubespray/) can help get a functional cluster bootstrapped.
  * Provider Pod will require admin authorization.

### Tools

Installed commands:

* [`akash`](https://github.com/ovrclk/akash/releases)
* [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl)
  * [`kustomize`](https://kubernetes-sigs.github.io/kustomize/installation/)
* [`jq`](https://stedolan.github.io/jq/)
* [`curl`](https://curl.haxx.se/)

### Working Directory

Create staging directory and cd into it

```sh
mkdir akash-demo && cd akash-demo
```

### Kubernetes Cluster

* Version: `1.19+`
* A Kubernetes cluster with your local `kubectl` with admin authentication and authorization.
* A wildcard A Record resolving to a cluster route-able IP. eg: `*.akashian.io`

#### Option 1: [Akash Kubespray](https://github.com/ovrclk/kubespray/) 

Akash has created an example Kubespray configuration to provision a Kubernetes installation on linux machines. Included are best practice configuration for CNI `calico` plugin, default Network Policies + Namespaces, `nginx` Ingress Controller, `metrics-server`, `cert-manager`, and Seccomp syscall restrictions.


#### Option 2: GCP Raw VM + k3s Example

*Warning: needs to be updated*

See [here](./kube-gce.md) for seting up these prerequisites
using [GCE](https://cloud.google.com/compute).

### Kubernetes Ingress

In this tutorial, we will be using the domain `akashian.io`.

```sh
PROVIDER_DOMAIN=akashian.io
```

This `PROVIDER_DOMAIN` variable is used to configure the Kubernetes Ingress Controller domain, so for this example, a Deployment's requests will be routed via a random subdomain, eg: `kswtibraxfdhlflhrg6ahe.akashian.io`. A wildcard subdomain DNS entry is necessary to support this, eg `*.akashian.io`. This address can be customized in the [Configure provider services](#configure-provider-services) file's `ingress-domain` field, eg: `ingress-domain=ingress.akashian.io`.

### Akash Network

We'll be connecting to the testnet.

```sh
export AKASH_NODE=tcp://rpc-edgenet.akashdev.net:26657
export AKASH_CHAIN_ID=edgenet
export AKASH_KEYRING_BACKEND=test
export AKASH_PROVIDER_KEY=provider
```

## Deploy Provider Services

### Download `akash` binary

```sh
curl -sSfL https://raw.githubusercontent.com/ovrclk/akash/master/godownloader.sh | sh
```

### Configure Akash Client

```sh
export AKASH_HOME="$PWD/home"
mkdir -p "$AKASH_HOME"
```

### Create Akash Provider Key

```sh
akash keys add $AKASH_PROVIDER_KEY --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND 
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
akash keys show provider -a --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND 
```

You can fund the address at the testnet [faucet](https://akash.vitwit.com/faucet).

Ensure you have funds with:

```sh
akash query bank balances --home=$AKASH_HOME --node=$AKASH_NODE "$(akash keys show $AKASH_PROVIDER_KEY -a --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND )"
```

### Create Akash Provider

Register your provider on the Akash Network:

*Replace `--from` flag value with your own*

```sh
akash tx provider create provider.yaml --from $AKASH_PROVIDER_KEY --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND --node=$AKASH_NODE --chain-id=$AKASH_CHAIN_ID
```

### Configure provider services client

```sh
cat <<EOF > client-config.txt
node=$AKASH_NODE
chain-id=$AKASH_CHAIN_ID
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

**Replace "password" below with your own unique secret.**

```sh
echo "password" > key-pass.txt
(cat key-pass.txt; cat key-pass.txt) | akash --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND keys export $AKASH_PROVIDER_KEY 2> key.txt
```

### Configure Kubernetes Akash Namespace

Create the `akash-services` namespace for running Provider.

```sh
kubectl apply -f https://raw.githubusercontent.com/ovrclk/akash/master/_docs/kustomize/networking/namespace.yaml
```

### Configure Akash Kubernetes CRDs

```sh
kubectl apply -f https://raw.githubusercontent.com/ovrclk/akash/master/pkg/apis/akash.network/v1/crd.yaml
```

### View kubernetes configuration

```sh
kubectl kustomize .
```

### Install kubernetes configuration

```sh
kubectl kustomize . | kubectl apply -f-
```

### Check status of provider

```sh
akash provider status --node=$AKASH_NODE --provider "$(akash keys show $AKASH_PROVIDER_KEY -a --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND)"
```

## Deploy Demo Application

### Create key for deployment

```sh
./bin/akash keys add deploy --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND 
```

### Fund your deployment account

View your address with

```sh
./bin/akash keys show deploy -a --home=$AKASH_HOME --keyring-backend$AKASH_KEYRING_BACKEND 
```

You can fund the address at the testnet [faucet](https://akash.vitwit.com/faucet).

Ensure you have funds with:

```sh
./bin/akash query account --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND --node=$AKASH_NODE "$(./bin/akash keys show deploy -a  --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND )"
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
./bin/akash --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND --chain-id=$AKASH_CHAIN_ID --node=$AKASH_NODE --from deploy tx deployment create ./deployment.yaml 
```

### View Order

```sh
akash query market order list --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND --owner "$(akash keys show deploy -a --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND)"
```

### View Bids

```sh
akash query market bid list --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND --owner "$(akash keys show deploy -a --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND)"
```

### View Leases

```sh
akash query market lease list --node=$AKASH_NODE --owner "$(akash keys show deploy -a --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND)"
```

### Capture Deployment Sequence

__note:__ This can be set when creating the deployment.  It defaults to the
block height at that time.

```sh
DSEQ="$(akash query market lease list --node=$AKASH_NODE --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND  \
  --owner "$(akash keys show deploy -a --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND )" \
  | jq -r '.[0].id.dseq')"
```

### Send Manifest

```sh
akash provider send-manifest deployment.yaml --node=$AKASH_NODE \
  --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND \
  --dseq "$DSEQ" \
  --oseq 1 \
  --gseq 1 \
  --owner    "$(./bin/akash keys show deploy -a)" \
  --provider "$(./bin/akash keys show provider -a)"
```

### View Lease Status

```sh
akash provider lease-status --node=$AKASH_NODE \
  --home=$AKASH_HOME --keyring-backend=$AKASH_KEYRING_BACKEND \
  --dseq "$DSEQ" \
  --oseq 1 \
  --gseq 1 \
  --owner    "$(./bin/akash keys show deploy -a)" \
  --provider "$(./bin/akash keys show provider -a)"
```

### View Site

```sh
akash provider lease-status \
  --dseq "$DSEQ" \
  --oseq 1 \
  --gseq 1 \
  --owner    "$(./bin/akash keys show deploy -a)"     \
  --provider "$(./bin/akash keys show provider -a)" | \
  jq -r '.services[0].uris[0]' | \
  while read -r line; do 
    open "http://$line" 
  done
```

### Delete Deployment

```sh
akash tx deployment close \
  --node=$AKASH_NODE \
  --chain-id=$AKASH_CHAIN_ID \
  --home=$AKASH_HOME \
  --keyring-backend=$AKASH_KEYRING_BACKEND \
  --from $OWNER_ADDRESS \
  --dseq $DSEQ
```


