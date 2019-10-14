[![Akash](_docs/img/logo-label-black.svg "Akash")](#overview)

[![Build status](https://badge.buildkite.com/85e140e3e8c0257c63d976946b061b805f0f338cdca7b02a9c.svg?branch=master)](https://buildkite.com/ovrclk/akash)
[![Go Report Card](https://goreportcard.com/badge/github.com/ovrclk/akash)](https://goreportcard.com/report/github.com/ovrclk/akash)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

[Akash](https://akash.network) is a secure, transparent, and decentralized cloud computing marketplace that 
connects those who need computing resources (tenants) with those that have computing capacity to lease (providers).

For a high-level overview of the Akash protocol and network economics, 
check out the [whitepapers](https://akash.network/research); a detailed protocol definition can be 
found in the [design documentation](_docs/design.md); and the target workload definition spec is [here](_docs/sdl.md).

## Akash Suite

This repository contains Akash Suite, the reference implementation of the [Akash Protocol](https://akash.network/l/whitepaper).  

It is an actively-developed prototype currently focused on the distributed marketplace functionality.

The Akash Suite is composed of two applications: `akash` and `akashd`:

- `akashd` is the ([tendermint](https://github.com/tendermint/tendermint)-powered) blockchain node that
implements the decentralized exchange.
- `akash` is the client used to access the exchange and network
in general.

## Get Started

The easiest way to get started with akash is by trying Testnet. Sign up [here](https://akash.network/signup) to get started. 

# Installing

The [latest](https://github.com/ovrclk/akash/releases/latest) binary release can be installed with [Homebrew](https://brew.sh/):

```sh
$ brew tap ovrclk/tap
$ brew install akash
```

Or [GoDownloader](https://github.com/goreleaser/godownloader):

```sh
$ curl https://raw.githubusercontent.com/ovrclk/akash/master/godownloader.sh | sh
```

# Building

 * [Dependencies](#dependencies)
   * [MacOS](#macos)
   * [Arch Linux](#arch-linux)
 * [Akash Suite](#akash-suite)

## Dependencies

 Akash is developed and tested with [golang 1.11+](https://golang.org/).  Building requires a working [golang](https://golang.org/) installation, a properly set `GOPATH`, and `$GOPATH/bin` present in `$PATH`.

 Additional requirements are:

 * [glide](https://github.com/Masterminds/glide): Golang library management.

For development environments, requirements include:

 * [protocol buffers](https://developers.google.com/protocol-buffers/): Protobuf compiler.

 Most golang libraries will be packaged in the local `vendor/` directory via [glide](https://github.com/Masterminds/glide), however the following packages will
 be installed globally with their binaries placed in `$GOPATH/bin` by `make devdeps-install`:

 * [gogoprotobuf](https://github.com/gogo/protobuf): Golang protobuf compiler plugin.
 * [mockery](https://github.com/vektra/mockery): Mock generator.

 See below for dependency installation instructions for various platforms.

### MacOS:

```sh
brew install glide

# dev environment only:
brew install protobuf
```

### Arch Linux:

```sh
curl https://glide.sh/get | sh

# dev environment only:
sudo pacman -Sy protobuf
```

## Akash Suite

Download and build `akash` and `akashd`:

```sh
go get -d github.com/ovrclk/akash
cd $GOPATH/src/github.com/ovrclk/akash
make deps-install
make

# dev environment only:
make devdeps-install
```

# Running

We use thin integration testing environments to simplify
the development and testing process.  We currently have two environments:

* [Single node](_run/single): simple (no workloads) single node running locally.
* [Single node with workloads](_run/kube): single node and provider running locally, running workloads within a virtual machine.
* [Multi node](_run/multi): multiple nodes and providers running in a virtual machine.
