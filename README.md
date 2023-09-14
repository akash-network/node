# Akash - Decentralized Serverless Network

![tests](https://github.com/akash-network/node/workflows/tests/badge.svg)
[![codecov](https://codecov.io/github/akash-network/node/coverage.svg?branch=main)](https://codecov.io/github/akash-network/node?branch=main)

[![Go Report Card](https://goreportcard.com/badge/github.com/akash-network/node)](https://goreportcard.com/report/github.com/akash-network/node)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

[![Akash](_docs/img/logo-label-black.svg "Akash")](#overview)

[Akash](https://akash.network) is a secure, transparent, and decentralized cloud computing marketplace that connects those who need computing resources (tenants) with those that have computing capacity to lease (providers).

For a high-level overview of the Akash protocol and network economics, check out the [whitepaper](https://ipfs.io/ipfs/QmVwsi5kTrg7UcUEGi5UfdheVLBWoHjze2pHy4tLqYvLYv); a detailed protocol definition can be 
found in the [design documentation](https://docs.akash.network); and the target workload definition spec is [here](https://docs.akash.network/sdl).

# Branching and Versioning

The `main` branch contains new features and is under active development; the `mainnet/main` branch contains the current, stable release.

* **stable** releases will have even minor numbers ( `v0.8.0` ) and be cut from the `mainnet/main` branch.
* **unstable** releases will have odd minor numbers ( `v0.9.0` ) and be cut from the `main` branch.

## Akash Suite

Akash Suite is the reference implementation of the [Akash Protocol](https://ipfs.io/ipfs/QmdV52bF7j4utynJ6L11RgG93FuJiUmBH1i7pRD6NjUt6B). Akash is an actively-developed prototype currently focused on the distributed marketplace functionality.

The Suite is composed of one binary, `akash`, which contains a ([tendermint](https://github.com/tendermint/tendermint)-powered) blockchain node that
implements the decentralized exchange as well as client functionality to access the exchange and network data in general.

## Get Started with Akash

The easiest way to get started with Akash is by following the [Quick Start Guide](https://docs.akash.network/guides/deploy) to get started. 

## Join the Community

- [Join Developer Chat](https://discord.gg/6Rtn8aJkU4)
- [Become a validator](https://docs.akash.network/validating/validator)

## Official blog and documentation

- Read the documentation: [docs.akash.network](https://docs.akash.network)
- Send a PR or raise an issue for the docs [akash-network/docs](https://github.com/akash-network/docs)
- Read latest news and tutorials on the [Official Blog](https://blog.akash.network)

# Supported platforms

Platform | Arch | Status
--- | --- | :---
Darwin | amd64 | ✅ **Supported**
Darwin | arm64 | ✅ **Supported**
Linux | amd64 | ✅ **Supported**
Linux | arm64 (aka aarch64) | ✅ **Supported**
Linux | armhf GOARM=5,6,7 | ⚠️ **Not supported**
Windows | amd64 | ⚠️ **Experimental**

# Installing

The [latest](https://github.com/akash-network/node/releases/latest) binary release can be installed with [Homebrew](https://brew.sh/):

```sh
$ brew tap akash-network/tap
$ brew install akash
```

Or [GoDownloader](https://github.com/goreleaser/godownloader):

```sh
$ curl -sSfL https://raw.githubusercontent.com/akash-network/node/main/install.sh | sh
```

Or install a specific version with [GoDownloader](https://github.com/goreleaser/godownloader)

```sh
$ curl -sSfL https://raw.githubusercontent.com/akash-network/node/main/install | sh -s -- v0.22.0
```

# Roadmap and contributing

Akash is written in Golang and is Apache 2.0 licensed - contributions are welcomed whether that means providing feedback, testing existing and new feature or hacking on the source.

To become a contributor, please see the guide on [contributing](CONTRIBUTING.md)

## Development environment
[This doc](https://github.com/akash-network/node/blob/main/_docs/development-environment.md) guides through setting up local development environment

Akash is developed and tested with [golang 1.21.0+](https://golang.org/). 
Building requires a working [golang](https://golang.org/) installation, a properly set `GOPATH`, and `$GOPATH/bin` present in `$PATH`.
It is also required to have C/C++ compiler installed (gcc/clang) as there are C dependencies in use (libusb/libhid)
Akash build process and examples are heavily tied to Makefile.


## Building from Source
Command below will compile akash executable and put it into `.cache/bin`
```shell
make akash # akash is set as default target thus `make` is equal to `make akash`
```
once binary compiled it exempts system-wide installed akash within akash repo

## Running

We use thin integration testing environments to simplify
the development and testing process.  We currently have three environments:

* [Single node](_run/lite): simple (no workloads) single node running locally.
* [Single node with workloads](_run/single): single node and provider running locally, running workloads within a virtual machine.
* [full k8s](_run/kube): same as above but with node and provider running inside Kubernetes.
