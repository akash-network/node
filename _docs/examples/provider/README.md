# Example: Run Provider in a Kubernetes Environment

## Prerequisites

### 1. Kubernetes cluster with `kubectl` pointing to it

### 2. Route-able IP address and domain

We'll be using the domain `*.akashdemo.net`

```sh
export PROVIDER_DOMAIN=akashdemo.net
```

### Akash network to connect to

We'll be connecting to

|node|chain-id|
|---|---|
|tcp://sentry-1.akashtest.net:26657|testnet|

```sh
export AKASHCTL_NODE=tcp://sentry-1.akashtest.net:26657
export AKASHCTL_CHAIN_ID=testnet
```

## Steps

### Create staging directory and cd into it

```sh
mkdir akash-provider && cd akash-provider
```

### Download akashctl

```sh
curl -sSfL https://raw.githubusercontent.com/ovrclk/akash/master/godownloader.sh | sh
```

### Download Kustomize configuration

```sh
curl -s -o kustomization.yaml \
  https://raw.githubusercontent.com/ovrclk/akash/master/_docs/examples/provider/kustomization.yaml
```

### Download Akash SDL file

```sh
curl -s -o deployment.yaml \
  https://raw.githubusercontent.com/ovrclk/akash/master/_docs/examples/provider/deployment.yaml
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
```

### Create Akash key

```sh
./bin/akashctl keys add provider
```

### Configure Akash Provider

```sh
cat <<EOF > provider.yaml
host: http://provider.$PROVIDER_DOMAIN
attributes:
  - key: region
    value: us-west
```


### Create Akash Provider

Note: you may need tokens sent to your wallet to pay gas.

```sh
./bin/akashctl tx provider create provider.yaml --from provider
```

### Configure provider services client

```sh
cat <<EOF > client-config.txt
node=$AKASHCTL_NODE
chain-id=$AKASHCTL_CHAIN_ID
```

### Configure provider services

```sh
cat <<EOF > provider-config.txt
ingress-static-hosts=true
ingress-domain=$PROVIDER_DOMAIN
```

### Configure Provider gateway endpoint

This will configure the kubernetes ingress
to point to your provider.

```sh
cat <<EOF > gateway-host.yaml
- op: replace
  path: /spec/rules/0/host
  value: provider.$PROVIDER_DOMAIN
```

### Export keys

```sh
echo "password" > key-pass.txt
(cat key-pass.txt; cat key-pass.txt) | ./bin/akashctl keys export provider > key.txt
```

### View kubernetes configuration

```sh
kubectl kustomize .
```

### Install kubernetes configuration

```sh
kubectl kustomize . | kubectl apply -f-
```
