# setup

Start minikube
```sh
$ minikube start --cpus 4 --memory 4096
```

Initialize helm
```sh
$ helm init
```

Install docker image
```sh
$ make image-minikube
```

Generate genesis and config
```sh
$ make helm-init
```

Deploy nodes
```sh
$ make helm-install
```

Query master account
```sh
$ ./run.sh query master
```

Send tokenz
```sh
$ ./run.sh send
```

Query master account
```sh
$ ./run.sh query master
```

Query other account
```sh
$ ./run.sh query other
```
