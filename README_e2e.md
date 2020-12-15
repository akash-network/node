# End to End tests

This software contains end-to-end tests which can run in an automated manner. These tests try and cover as much of the
expected use cases as possible.

## Running with KIND

Prerequisites: You need docker installed & kind installed

This runs a Kubernetes cluster using KIND, which allows it to run entirely within Docker

Running:
```
cd _run/kube
make kind-cluster-create
cd ../..
make test-e2e-integration
```


Cleanup:
```
cd _run/kube
make kind-cluster-delete
```

## Running with K8S in Vagrant

Prerequisites:

1. Vagrant
2. VirtualBox
3. The ovrclk/kubespray project checked out at the same level as this project

This starts a complete K8S cluster that is provisioned by kubespray. This uses at least 6 gigabytes of system memory
when ran.

### Start the K8S cluster

Note this process can take a very long time (15 minutes+) even under ideal circumstances. There are multiple points
where Vagrant waits for a VM to be boot or to respond to SSH. If this fails, you can always run the stop script
then start over.

```
pushd ../kubespray && ./vagrant_up.sh && popd
```

The result of this is multiple VMs running under Virtualbox that form a complete kubernetes cluster. Your
kube config is automatically updated (the old file is backed up under ~/.kube) so you can do `kubectl get nodes`
to see the status of the nodes. Additionally, port 10080 on the host machine is forwarded to port 80 on the
master node, so the Kubernetes ingress controller is reachable at `localhost:10080`.

### Run the tests

```
make test-e2e-integration-k8s
```

### Stop the K8S cluster

This just destroys the VMs, so this should only take a few seconds


```
pushd ../kubespray && ./vagrant_down.sh && popd
```

