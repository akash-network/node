# Photon [![Build Status](https://travis-ci.com/ovrclk/photon.svg?token=xMx9pPujMteGc5JpGjzX&branch=master-update)](https://travis-ci.com/ovrclk/photon) [![Coverage Status](https://coveralls.io/repos/github/ovrclk/photon/badge.svg?t=bXSvuV)](https://coveralls.io/github/ovrclk/photon)

The Photon Network is a blockchain-powered cloud infrasture system that pairs independent datacenter providers with users seeking high-performance application hosting.  The process is simple for both parties - Datacenter components are easy to install and provide a high degree of automation, while application deployment and administration is simple and intuitive.

* [Overview](#overview)
* [Design](#design)
  * [Users](#users)
  * [Datacenters](#datacenters)
  * [Distributed Exchange](#distributed-exchange)
* [Building](#building)

## Overview

The foundational design objective of the Photon Network is to maintain a low barrier to entry for
providers while at the same time ensuring that clients can trust the resources that the platform
offers them.  To achieve this, the system requires a publicly-verifiable record of transactions
within the network.  To that end, the Photon Network is implemented using blockchain technologies as
a means of achieving consensus on the veracity of a distributed database.

Photon is, first and foremost, a platform that allows clients to procure resources from providers.
This is enabled by a blockchain-powered distributed exchange where clients post their desired
resources for providers to bid on.  The currency of this marketplace is a digital token, the Photon
(PTN), whose ledger is stored on the blockchain-based distributed database.

Photon is a cloud platform for real-world applications. The requirements of such applications
include:

* Many workloads deployed across any number of datacenters.
* Connectivity restrictions which prevent unwanted access to workloads.
* Self-managed so that operators do not need to constantly tend to deployments.

To support running workloads on procured resources, Photon includes a peer-to-peer protocol for
distributing workloads and deployment configuration to and between a client’s providers.

Workloads in Photon are defined as Docker containers.  Docker containers allow for highly-isolated
and configurable execution environments, and are already part of many cloud-based deployments today.

## Design

Comprehensive design documentation can be found [here](_docs/design.md).

### Users

A user hosting an application on the Photon network.

Initially, users interact with the network through a [command-line inteface](_docs/photon-cli.md) and define their deployments via a [declarative file](_docs/sdl.md).

### Datacenters

Each datacenter will host an agent which is a mediator between the with the Photon Network and datecenter-local infrastructure.

The datacenter agent is responsible for:

* Bidding on deployment orders fulfillable by the datacenter.
* Managing managing active leases it is a provider for.

### Distributed Exchange

Users lease resources from [datacenters] through a distributed exchange.  The Photon protocol enables this exchange by providing a distributed
orderbook and a set of transactions to act upon it.

### Workload Distribution

## Building

 * [Dependencies](#dependencies)
   * [MacOS](#macos)
   * [Arch Linux](#arch-linux)
 * [Building Photon](#photon-1)
 * [Testing](#testing)
 * [Documentation](#documentation)

### Dependencies

 Photon is developed and tested with [golang 1.8+](https://golang.org/).  Building requires a working [golang](https://golang.org/) installation, a properly set `GOPATH`, and `$GOPATH/bin` present in `$PATH`.

 Additional requirements are:

 * [glide](https://github.com/Masterminds/glide): Golang library management.

For development environments, requirements include:

 * [protocol buffers](https://developers.google.com/protocol-buffers/): Protobuf compiler.

 Most golang libraries will be packaged in the local `vendor/` directory via [glide](https://github.com/Masterminds/glide), however the following packages will
 be installed globally with their binaries placed in `$GOPATH/bin` by `make devdeps-install`:

 * [protoc-gen-go](https://github.com/golang/protobuf): Golang protobuf compiler plugin.
 * [`cfssl`,`cfssljson`,`mkbundle`](https://github.com/cloudflare/cfssl): CFSSL command-line utilities.

 See below for dependency installation instructions for various platforms.

#### MacOS:

```sh
brew install glide

# dev environment only:
brew install protobuf
```

#### Arch Linux:

```sh
curl https://glide.sh/get | sh

# dev environment only:
sudo pacman -Sy protobuf
```

### Photon

Download and build photon:

```sh
go get -d github.com/ovrclk/photon
cd $GOPATH/src/github.com/ovrclk/photon
make deps-install
make

# dev environment only:
make devdeps-install
```

### Testing

```sh
make test
make test-full
```

### Documentation

```sh
make docs
```
