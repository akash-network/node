# Provider hostname management script

This script migrates from the v0.12.x provider hostname management to the v0.14.x provider hostname management. This is accomplished by doing the following.

1. Scanning all existing ingress objects in Kuberenetes created by the provider
2. Creating ProviderHost CRD entries in the akash namespace based off those entries
3. Removing all existing ingress objects in Kubernetes created by the provider

There is no step to recreate the ingress objects. The hostname operator does this automatically on first start.

# Dependencies

1. Python 3.x
2. `kubectl` command line tool
3. Set the environment variable `KUBECONFIG` to connect to your provider's kubernetes cluster.

## Example usage

*Step 1*: Back up everything currently created by akash a network ingress

```
kubectl get ing -A -l akash.network -o json > ing-backup.json
```

This is meant to be a stopgap measure.

*Step 2*: Stop your provider. Your provider should be offline at this point. Deployments remaining running in Kubernetes

*Step 3*: Run `python3 provider_migrate_to_hostname_operator.py backup`. This creates two files. The first file is `provider_hosts.pickle` which is the data used to rebuild
the objects later. The second file is `ingresses_backup.json` which is just a raw backup of each ingress object as retrieved from Kubernetes

*Step 4*:

Apply provider host CRD stored in `pkg/apis/akash.network/provider_hosts_crd.yaml` is applied to your kubernetes cluster by running

```
kubectl apply -f pkg/apis/akash.network/provider_hosts_crd.yaml
```

Apply the newest ingress controller stored in `_run/ingress-nginx.yaml`

```
kubectl apply -f _run/ingress-nginx.yaml
```

Apply the newest ingress class stored in `_run/ingress-nginx.yaml`

```
kubectl apply -f _run/ingress-nginx-class.yaml
```

*Step 5*: Run `python3 provider_migrate_to_hostname_operator.py create`. This parses the data and adds the provider hosts entries in kubernetes.

*Step 6*: Run `python3 provider_migrate_to_hostname_operator.py purge`. This removes all the ingress objects from kubernetes.

*Step 7*: Install the new kubernetes hostname operator, which manages hostnames going forward.

The hostname operator is implemented in `_docs/kustomize/akash-hostname-operator`. It normally uses the latest version of the docker container image but you should specify the version you are deploying. This is done by editing `_docs/kustomize/akash-hostname-operator/kustomization.yaml` and appending the following section

```
images:
  - name: ghcr.io/ovrclk/akash:stable
    newName: ghcr.io/ovrclk/akash
    newTag: v0.14.0
```

The last line specifies the image tag and should correspond to whatever version you are installing.

To install the operator into kubernetes perform the following from the `/_docs` directory.

```
kubectl kustomize ./kustomize/akash-hostname-operator | kubectl apply -f -
```
