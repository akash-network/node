# Testing

We use a number of different testing strategies.

1. [Unit testing](#unit-tests)
1. [K8S Integration Tests](#k8s-integration-tests)

There are a number of different testing strategies

## Unit Tests

Basic unit/functional testing.  External dependencies are mocked

To honor cached tests, simply run:

```
make test
```

To force test everything available, run

```
make test-nocache
```

## K8S Integration Tests

These test our integration with Kubernetes.  They use small, ephemeral Kubernetes
clusters set up from [kind](https://kind.sigs.k8s.io/).

### Set up cluster

```sh
kind create cluster
./script/setup-kind.sh
```

### Run tests

```go
make test-k8s-integration
```

### Snippets

If things are ever behaving strangly, you can remove everything installed by akash
with:

```sh
kubectl delete ns -l akash.network
```

or, as a last resort, delete and re-create the cluster:

```sh
kind delete cluster
kind create cluster
./script/setup-kind.sh
```
