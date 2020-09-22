Akash Provider's implementation of Kubernetes Hard Multi-Tenancy
------------------------------------------------

The Kubernetes Provider needs to initialize reasonable security protections against malicious threats from tenants against the provider and other tenants. Kubernetes does not provide perfect multi-tenant threat encapsulation, as do most container orchestrators, but using reasonable default controls and privileges; we aim to provide the best possible security protections for Providers and Tenants on the Akash Network. This will involve multiple layers of protection around separate threat domains.

## Domain: Container/CGroups/Kernel/Service Accounts

Tenant containers are not allowed to run with root privileges on the host machine(for now). There are far too many exploits related to Containers CGroups to trust them to not be exploited. So the Provider applies a "Restrictive PodSecurityPolicy" to all tenant containers; revoking root privileges, disabling certain sys-calls know to lead to escalation of privilege, denying the Pod a service account token, blocking access to root file system, and only allowing applicable volumes.

There are use cases where running as root might be necessary, and the provider with appropriate consideration and controls would allow such operations. eg: Workloads which interface the GPU. This sort of risk needs to be managed by the providers and will be part of their selectable attributes. By default, the Akash Provider blocks workloads running as root with PodSecuirtyPolicy(see below), however this feature will be enabled in the future, and must be enabled by the Provider.

### Pod Security Policies

Restricted PSP source: https://raw.githubusercontent.com/kubernetes/website/master/content/en/examples/policy/restricted-psp.yaml

These polices are applied by the Provider in [provider/cluster/kube/builder.go](https://github.com/ovrclk/akash/blob/master/provider/cluster/kube/builder.go).

```
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: restricted
  annotations:
    seccomp.security.alpha.kubernetes.io/allowedProfileNames: 'docker/default,runtime/default'
    apparmor.security.beta.kubernetes.io/allowedProfileNames: 'runtime/default'
    seccomp.security.alpha.kubernetes.io/defaultProfileName:  'runtime/default'
    apparmor.security.beta.kubernetes.io/defaultProfileName:  'runtime/default'
spec:
  privileged: false
  # Required to prevent escalations to root.
  allowPrivilegeEscalation: false
  # This is redundant with non-root + disallow privilege escalation,
  # but we can provide it for defense in depth.
  requiredDropCapabilities:
    - ALL
  # Allow core volume types.
  volumes:
    - 'configMap'
    - 'emptyDir'
    - 'projected'
    - 'secret'
    - 'downwardAPI'
    # Assume that persistentVolumes set up by the cluster admin are safe to use.
    - 'persistentVolumeClaim'
  hostNetwork: false
  hostIPC: false
  hostPID: false
  runAsUser:
    # Require the container to run without root privileges.
    rule: 'MustRunAsNonRoot'
  seLinux:
    # This policy assumes the nodes are using AppArmor rather than SELinux.
    rule: 'RunAsAny'
  supplementalGroups:
    rule: 'MustRunAs'
    ranges:
      # Forbid adding the root group.
      - min: 1
        max: 65535
  fsGroup:
    rule: 'MustRunAs'
    ranges:
      # Forbid adding the root group.
      - min: 1
        max: 65535
  readOnlyRootFilesystem: false
```

This policy is applied by the Provider when the Tenant's K8s Namespace is created,.

## Domain: Networking

Networking connections the tenant containers are allowed to create. Ideal goal is that tenant Containers should only be able to connect to fellow Pod Containers within their Namespace, and the open web. They are ideally not allowed to Egress, or create a connection, to other tenant namespaces, system utilities, or Kubernetes API. Kubernetes NetworkPolicies are put into affect to support these restrictions, but Networking Plugin should be tested to assert rules are effective given the Provider's environment.

### Network Policies(per Tenant Namespace)

Kubernetes Networking Policies enforced by a [CNI Plugin](https://kubernetes.io/docs/tasks/administer-cluster/network-policy-provider/calico-network-policy/)([Calico](https://docs.projectcalico.org/getting-started/kubernetes/) is recommended) 

These polices are applied by the Provider in [provider/cluster/kube/builder.go](https://github.com/ovrclk/akash/blob/master/provider/cluster/kube/builder.go).

### Ingress Rules
* Deny all Ingress into Tenant namespace. This is the base for providing controlled access to the Namespace.
* Allow Ingress traffic from containers withing the Tenant's Namespace.(Inter container traffic)
* Allow Ingress traffic from Ingress Controller(ingress-nginx for now, but should be made to reference generic ingress controllers).

### Egress Rules
* Deny All Egress from Tenant namespace by default.
* Allow Egress Traffic to other pods within the Tenant Namespace.
* Allow Egress Traffic to all addresses 0.0.0.0/0 EXCEPT: 10.0.0.0/8(Internal Subnet of Cluster).
  * TODO: Testing indicates this assertion does work as expected. Connections from the tenant namespace are able to open connections to the Kubernetes API, which is what this rule attempts to block.
    * The previous two rules are in effect, and connections to Internet are blocked until this rule is applied, however it does not exclude preventing connections to 10.0.0.0/8 as expected.
    * Due to the PodSecurityPolicy in place, the container has no service account or AuthZ|N with the API, but blocking access entirely would be ideal.
* Allow Egress Traffic `UDP Port:53` to the `coredns` deployments hosted in the `kube-system` namespace.

### Debugging YAML Declarations of Provider Applied Egress NetPols

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-egress
  namespace: TODO
spec:
  podSelector:
    matchLabels: {}
  policyTypes:
  - Egress
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: egress-all-but-internal
  namespace: TODO
spec:
  podSelector: {}
  egress:
  - to:
    - ipBlock:
        cidr: 0.0.0.0/0
        except:
          - 10.0.0.0/8
  policyTypes:
  - Egress
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-dns-access
  namespace: TODO
spec:
  podSelector:
    matchLabels: {}
  policyTypes:
  - Egress
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: UDP
      port: 53

```

#### Debugging Pod

Inject this pod into a Tenant or general namespace to get a shell and assert that network rules are being enforced. Note the lack of `hostNetwork: true` Pod `spec` attribute, if set that will bypass all CNI set rules.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: shell-demo
  namespace: TODO
spec:
  volumes:
  - name: shared-data
    emptyDir: {}
  containers:
  - name: nginx
    image: nginx:latest
    volumeMounts:
    - name: shared-data
      mountPath: /usr/share/nginx/html
```

Origin from Kubernetes [documentation.](https://kubernetes.io/docs/tasks/debug-application-cluster/get-shell-running-container/)

## Resources

* [Kubernetes Network Policy Docs](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
    * [Debugging Services Docs](https://kubernetes.io/docs/tasks/debug-application-cluster/debug-service/)
* [Security Boulevard's introduction to Network Policies](https://securityboulevard.com/2020/03/an-introduction-to-kubernetes-network-policies-for-security-people/)
    * Good examples of various Networking Policies
* Jessie Frazzelle's K8s [Hard Multi-Tenancy](https://blog.jessfraz.com/post/hard-multi-tenancy-in-kubernetes/) (2018)
    * Updated [Multi-Tenant Orchestrator](https://blog.jessfraz.com/post/secret-design-docs-multi-tenant-orchestrator/)
* Kubernetes [Working Group](https://groups.google.com/g/kubernetes-wg-multitenancy) on Multi-Tenancy
    * Hierarch Namespace Controller [HNC](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/incubator/hnc) (In progress)
* Bust-a-kube attack/defense [resources](https://www.bustakube.com/learning-resources)
* Network Policy [Visualizer](https://orca.tufin.io/netpol/)
* CIDR Notation [Calculator](https://www.ipaddressguide.com/cidr)

### General Example Network Policies
```yaml
 # Default Deny All Policy
kind: NetworkPolicy
apiVersion: networking.k8s.io/v1
metadata:
  name: default-deny-all
spec:
  podSelector: {}
  policyTypes:
    - Ingress
    - Egress
---
kind: NetworkPolicy
apiVersion: networking.k8s.io/v1
metadata:
  name: allow-internet-only
spec:
  podSelector: {}
  policyTypes:
    - Egress
  egress:
    - to:
      - ipBlock:
        cidr: 0.0.0.0/0
           except:
               - 10.0.0.0/8
               - 192.168.0.0/16
               - 172.16.0.0/20
```