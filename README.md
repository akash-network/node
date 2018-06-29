[![Akash](_docs/img/logo-label-black.svg "Akash")](#overview)
[![Build status](https://badge.buildkite.com/85e140e3e8c0257c63d976946b061b805f0f338cdca7b02a9c.svg)](https://buildkite.com/ovrclk/akash)
[![Coverage](https://codecov.io/gh/ovrclk/akash/branch/master/graph/badge.svg)](https://codecov.io/gh/ovrclk/akash)
[![Go Report Card](https://goreportcard.com/badge/github.com/ovrclk/akash)](https://goreportcard.com/report/github.com/ovrclk/akash)

# Overview

Akash is a cloud infrastructure platform whose resources are provided
by independent datacenters.  A high-level overview of the Akash Protocol
can be found [here](https://akash.network/paper.pdf); a detailed
protocol definition can be found [here](_docs/design.md); and the target
workload definition spec is [here](_docs/sdl.md).

This repository contains Akash Suite, the reference implementation of the
[Akash Protocol](https://akash.network/paper.pdf).  It is an actively-developed
prototype currently focused on the distributed marketplace functionality.

The Akash Suite is composed of two applications: `akash` and `akashd`.  `akashd`
is the ([tendermint](https://github.com/tendermint/tendermint)-powered) blockchain node that
implements the decentralized exchange; `akash` is the client used to access the exchange and network
in general.

# Building

 * [Dependencies](#dependencies)
   * [MacOS](#macos)
   * [Arch Linux](#arch-linux)
 * [Akash Suite](#akash-suite)

## Dependencies

 Akash is developed and tested with [golang 1.8+](https://golang.org/).  Building requires a working [golang](https://golang.org/) installation, a properly set `GOPATH`, and `$GOPATH/bin` present in `$PATH`.

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

* [Single node](_run/single): simple single node running locally
* [Multi node](_run/multi): multi-node setup within a virtual machine.

Each of these environments can demonstrate:

* Sending tokens from one account to another.
* Creating provider accounts.
* Running a provider which bids on open orders.
* Creating deployments.
* Obtaining leases for deployments.
* Monitoring marketplace activity.
* Querying details of all objects.
