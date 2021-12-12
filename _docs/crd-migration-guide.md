# CRDs migration guide for provider from v0.14.x to TBD

[Version](#version) will introduce changes in CRDs that need to be applied in order to continue operations.

All steps below MUST be executed in same order as declared and applies to your environment.
If any step fails STOP and reach out to Akash team

## Provider service is running on bare-metal server
1. Stop provider service. Most probably it is running by `systemd`
2. Upgrade RPC node to [Version](#version). If you are using 3rd-party RPC node ensure upgrade has been applied
3. Download and install **link to release**
4. Install new CRDs
```shell
kubectl apply -f https://raw.githubusercontent.com/ovrclk/akash/v0.16.0/pkg/apis/akash.network/crd.yaml
kubectl apply -f https://raw.githubusercontent.com/ovrclk/akash/v0.16.0/pkg/apis/akash.network/provider_hosts_crd.yaml
``` 
5. Perform migration
```shell
akash provider migrate crds
```
6. Start provider service

## Provider service is running in kubernetes
1. Stop provider service. Scale provider deployment to `0`
2. Upgrade RPC node to [Version](#version). If you are using 3rd-party RPC node ensure upgrade has been applied
3. Prepare kubeconfig for cluster in either ~/.kube/config or have `KUBECONFIG` environment variable set. **Skip this step if provider service running under `systemd` on bare-metal.**
4. Test access to the cluster using `kubectl`
5. Install new CRDs
```shell
kubectl apply -f https://raw.githubusercontent.com/ovrclk/akash/v0.16.0/pkg/apis/akash.network/crd.yaml
kubectl apply -f https://raw.githubusercontent.com/ovrclk/akash/v0.16.0/pkg/apis/akash.network/provider_hosts_crd.yaml
``` 
6. Download and install **link to release** on your local machine
7. Perform migration
```shell
akash provider migrate crds
```
8. Upgrade provider service in kubernetes

## Notes

Migrate command creates directory to backup kubernetes objects. By default it is current directory. It can be changed with `--k8s-crd-migrate-path` flag
Directory content is preserved after successful. Delete it IF and ONLY IF migrate has been successful

## Example output of the upgrade
```
using kube config file /Users/amr/.kube/config
checking manifests to backup
checking providers hosts to backup
total to backup
        manifests:      1
        provider hosts: 2
filtering closed leases
backup manifests
backup manifests DONE
backup provider hosts
backup provider hosts DONE
applying manifests
manifest "0l4sg41jiahbrp5sbsb6r8ccq3nd7i7nla6qfk1c7dt6q" has been migrated successfully
applying manifests  DONE
applying hosts
provider host "hello.localhost" has been migrated successfully
provider host "ppg85nclo99tdc5rgmu7eb0n8c.localhost" has been migrated successfully
applying hosts      DONE
```

##Version
`TBD`
