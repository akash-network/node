# Akash - Decentralized Serverless Network

![tests](https://github.com/ovrclk/akash/workflows/tests/badge.svg)
![simulations](https://github.com/ovrclk/akash/workflows/Sims/badge.svg)
[![codecov](https://codecov.io/github/ovrclk/akash/coverage.svg?branch=master)](https://codecov.io/github/ovrclk/akash?branch=master)

[![Go Report Card](https://goreportcard.com/badge/github.com/ovrclk/akash)](https://goreportcard.com/report/github.com/ovrclk/akash)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

[![Akash](_docs/img/logo-label-black.svg "Akash")](#overview)

[Akash](https://akash.network) is a secure, transparent, and decentralized cloud computing marketplace that connects those who need computing resources (tenants) with those that have computing capacity to lease (providers).

For a high-level overview of the Akash protocol and network economics, check out the [whitepaper](https://akash-web-prod.s3.amazonaws.com/uploads/2020/03/akash-econ.pdf); a detailed protocol definition can be 
found in the [design documentation](https://docs.akash.network); and the target workload definition spec is [here](https://docs.akash.network/sdl).

# Branching and Versioning

The `master` branch contains new features and is under active development; the `mainnet/main` branch contains the current, stable release.

* **stable** releases will have even minor numbers ( `v0.8.0` ) and be cut from the `mainnet/main` branch.
* **unstable** releases will have odd minor numbers ( `v0.9.0` ) and be cut from the `master` branch.

## Akash Suite

Akash Suite is the reference implementation of the [Akash Protocol](https://akash-web-prod.s3.amazonaws.com/uploads/2020/03/akash-econ.pdf). Akash is an actively-developed prototype currently focused on the distributed marketplace functionality.

The Suite is composed of one binary, `akash`, which contains a ([tendermint](https://github.com/tendermint/tendermint)-powered) blockchain node that
implements the decentralized exchange as well as client functionality to access the exchange and network data in general.

## Get Started with Akash

The easiest way to get started with Akash is by following the [Quick Start Guide](https://docs.akash.network/guides/deploy) to get started. 

## Join the Community

- [Join Developer Chat](https://discord.gg/6Rtn8aJkU4)
- [Become a validator](https://akash.network/token)

## Official blog and documentation

- Read the documentation: [docs.akash.network](https://docs.akash.network)
- Send a PR or raise an issue for the docs [ovrclk/docs](https://github.com/ovrclk/docs)
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

## Development environment
Akash is developed and tested with [golang 1.16.0+](https://golang.org/). 
Building requires a working [golang](https://golang.org/) installation, a properly set `GOPATH`, and `$GOPATH/bin` present in `$PATH`.
It is also required to have C/C++ compiler installed (gcc/clang) as there are C dependencies in use (libusb/libhid)
Akash build process and examples are heavily tied to Makefile.

Akash also uses [direnv](https://direnv.net) to setup and seamlessly update environment. List of variables exported in root dir are listed in [.env](./.env)
It sets local dir `.cache` to hold all temporary files and tools (except **kind** which installed ) required for development purposes.
It is possible to set custom path to `.cache` with `AKASH_DEVCACHE` environment variable.
All tools are referred as `makefile targets` and set as dependencies thus installed (to `.cache/bin`) only upon necessity.
For example `protoc` installed only when `proto-gen` target called.

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
