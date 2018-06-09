# Akash: Single-Node Local Setup with Minikube

Run a network with a single, local node and execute workloads in Minikube.

Running through the entire suite requires four terminals.
Each command is marked __t1__-__t4__ to indicate a suggested terminal number.

## Setup

__t1__: Start and initialize minikube
```sh
$ minikube start
$ minikube addons enable ingress
$ kubectl create -f contour.yml
```

__t1__: Build binaries
```sh
$ make
```

__t1__: Initialize environment
```sh
$ ./run.sh init
```

## Start Network

__t2__: Run `akashd`
```sh
$ ./run.sh akashd
```

## Transfer Tokens

__t1__: Query _master_ account
```sh
$ ./run.sh query master
```

__t1__: Send tokens to _other_ account
```sh
$ ./run.sh send
```

__t1__: Query _master_ account
```sh
$ ./run.sh query master
```

__t1__: Query _other_ account
```sh
$ ./run.sh query other
```

## Marketplace

__t3__: Start marketplace monitor
```sh
$ ./run.sh marketplace
```

__t4__: Run provider
```sh
$ ./run.sh provider
```

__t1__: Create Deployment
```sh
$ ./run.sh deploy
```

__t1__: Ping workload
```sh
$ ./run.sh ping
```
