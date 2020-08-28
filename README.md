# Akash - Decentralized Serverless Network

![tests](https://github.com/ovrclk/akash/workflows/tests/badge.svg)
![simulations](https://github.com/ovrclk/akash/workflows/Sims/badge.svg)
[![codecov](https://codecov.io/github/ovrclk/akash/coverage.svg?branch=master)](https://codecov.io/github/ovrclk/akash?branch=master)

[![Go Report Card](https://goreportcard.com/badge/github.com/ovrclk/akash)](https://goreportcard.com/report/github.com/ovrclk/akash)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

[![Akash](_docs/img/logo-label-black.svg "Akash")](#overview)

[Akash](https://akash.network) is a secure, transparent, and decentralized cloud computing marketplace that connects those who need computing resources (tenants) with those that have computing capacity to lease (providers).

For a high-level overview of the Akash protocol and network economics, check out the [whitepapers](https://akash.network/research); a detailed protocol definition can be 
found in the [design documentation](_docs/design.md); and the target workload definition spec is [here](_docs/sdl.md).

# NOTE

The `master` branch is currently unstable as we migrate to [cosmos-sdk](https://github.com/cosmos/cosmos-sdk) from the previous tendermint-core implementation, and to subsequent protobuf RPC implementation. The previous, stable branch can be found at [stable](https://github.com/ovrclk/akash/tree/stable).

## Akash Suite

Akash Suite is the reference implementation of the [Akash Protocol](https://akash.network/l/whitepaper). Akash is an actively-developed prototype currently focused on the distributed marketplace functionality.

The Suite is composed of one binary, `akash`, which contains a ([tendermint](https://github.com/tendermint/tendermint)-powered) blockchain node that
implements the decentralized exchange as well as client functionality to access the exchange and network data in general.

## Get Started with Akash

The easiest way to get started with Akash is by trying Testnet. Sign up [here](https://akash.network/signup) to get started. 

## Join the Community

- [Join Developer Chat](https://akash.network/chat)
- [Become a validator](https://akash.network/token)

## Official blog and documentation

- Read the documentation: [docs.akash.network](https://docs.akash.network)
- Send a PR or raise an issue for the docs [ovrclk/docs](https://github.com/ovrclk/docs)
- Read latest news and tutorials on the [Official Blog](https://blog.akash.network)

# Installing

The [latest](https://github.com/ovrclk/akash/releases/latest) binary release can be installed with [Homebrew](https://brew.sh/):

```sh
$ brew tap ovrclk/tap
$ brew install akash
```

Or [GoDownloader](https://github.com/goreleaser/godownloader):

```sh
$ curl -sSfL https://raw.githubusercontent.com/ovrclk/akash/master/godownloader.sh | sh
```

Or install a specific version with [GoDownloader](https://github.com/goreleaser/godownloader)

```sh
$ curl -sSfL https://raw.githubusercontent.com/ovrclk/akash/master/godownloader.sh | sh -s -- v0.7.8
```

# Roadmap and contributing

Akash is written in Golang and is Apache 2.0 licensed - contributions are welcomed whether that means providing feedback, testing existing and new feature or hacking on the source.

To become a contributor, please see the guide on [contributing](CONTRIBUTING.md)

## Building from Source

 * [Dependencies](#dependencies)
   * [MacOS](#macos)
   * [Arch Linux](#arch-linux)
 * [Akash Suite](#akash-suite)

### Dependencies

Akash is developed and tested with [golang 1.13.1+](https://golang.org/).  Building requires a working [golang](https://golang.org/) installation, a properly set `GOPATH`, and `$GOPATH/bin` present in `$PATH`.

For development environments, requirements include:

 * [protocol buffers](https://developers.google.com/protocol-buffers/): Protobuf compiler.

 Most golang libraries will be installed via [`go modules`](https://github.com/golang/go/wiki/Modules),
 however the following packages will
 be installed globally with their binaries placed in `$GOPATH/bin` by `make devdeps-install`:

 * [gogoprotobuf](https://github.com/gogo/protobuf): Golang protobuf compiler plugin.
 * [mockery](https://github.com/vektra/mockery): Mock generator.
 * [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway): HTTP<->gRPC proxy.
 * [golangci-lint](https://github.com/golangci/golangci-lint): golang static analysis tool.

 See below for dependency installation instructions for various platforms.

#### MacOS:

```sh
# dev environment only:
brew install protobuf
```

#### Arch Linux:

```sh
# dev environment only:
sudo pacman -Sy protobuf
```
### Akash Suite

Download and build `akash`:

```sh
go get -d github.com/ovrclk/akash
cd $GOPATH/src/github.com/ovrclk/akash
make deps-install
make

# dev environment only:
make devdeps-install
```

## Running

We use thin integration testing environments to simplify
the development and testing process.  We currently have two environments:

* [Single node](_run/single): simple (no workloads) single node running locally.
* [Single node with workloads](_run/kube): single node and provider running locally, running workloads within a virtual machine.
* [Multi node](_run/multi): multiple nodes and providers running in a virtual machine.
