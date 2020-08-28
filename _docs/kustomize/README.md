Kustomize Kubernetes
--------------------

Directory contains templates and files to configure a Kubernetes cluster to support Akash Provider services.

## `networking/`

Normal Kubernetes declaration files to be `kubectl apply -f networking/`

* `akash-services` namespace declaration
* Default-deny Network policies for the `default` namespace to cripple the potential of malicious containers being run there.

## `akash-services/`

[Kustomize](https://kubernetes.io/docs/tasks/manage-kubernetes-objects/kustomization/) directory for configuring Network Policies to support `akash-services` namespace of Akash apps.

`kubectl kustomize akash-services/ | kubectl apply -f -`

## `akashd/` 

Kustomize directory to configure running the `akash` blockchain node service.

`kubectl kustomize akashd/ | kubectl apply -f -`

## `akash-provider/`

Kustomize directory to configure running the `akash-provider` instance which manages tenant workloads within the cluster.

`kubectl kustomize akash-provider/ | kubectl apply -f -`
