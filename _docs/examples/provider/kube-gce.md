# Kubernetes cluster on GCE

Follow these steps to create a Kubernetes cluster on
[GCE](https://cloud.google.com/compute) using [`gcloud`](https://cloud.google.com/sdk/gcloud).

TODO: update to Kubespray. The K3S portions of this are based off of [this](https://starkandwayne.com/blog/trying-tiny-k3s-on-google-cloud-with-k3sup/)
tutorial.

## Requirements

* [`gcloud`](https://cloud.google.com/sdk/gcloud) command installed.
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) command installed.
* [GCP](https://cloud.google.com/) account and project.
* [Cloud DNS](https://cloud.google.com/dns) enabled for project (instructions [here](https://cloud.google.com/dns/docs/tutorials/create-domain-tutorial)).

## Steps

### Prepare Settings

```sh
GCLOUD_PROJECT=testnet-provider
GCLOUD_DNS_ZONE=akashian-io
GCLOUD_DNS_DOMAIN=akashian.io
```

Configure `gcloud` project and default regions

```sh
gcloud config set project "$GCLOUD_PROJECT"
gcloud compute project-info add-metadata \
    --metadata google-compute-default-region=us-west1,google-compute-default-zone=us-west1-a
```

### Create Instance

```sh
gcloud compute instances create p1 \
    --machine-type n1-standard-1 \
    --tags k3s,k3s-master
```

### Download `k3sup`

```sh
curl -sLS https://get.k3sup.dev | sh
```

On Linux, install the binary to somewhere in your path, or use `./k3sup`:

```sh
sudo install k3sup /usr/local/bin
```

### Setup remote settings

```sh
gcloud compute config-ssh
```

```sh
INSTANCE_PUBLIC_IP="$(gcloud compute instances list \
  --filter="name=(p1)" \
  --format="get(networkInterfaces[0].accessConfigs.natIP)")"
```

### Create Kubernetes cluster

```sh
k3sup install --ip "$INSTANCE_PUBLIC_IP" --context k3s --ssh-key ~/.ssh/google_compute_engine --user "$(whoami)"
```

### Configure firewall rules

```sh
gcloud compute firewall-rules create k3s          --allow=tcp:6443 --target-tags=k3s
gcloud compute firewall-rules create inbound-http --allow=tcp:80   --target-tags=k3s
```

### Configure kubectl

```sh
export KUBECONFIG=$PWD/kubeconfig
```

### Test connectivity

```sh
kubectl get pods  -A
kubectl top nodes
```

### Configure DNS

```sh
gcloud dns record-sets transaction start --zone="$GCLOUD_DNS_ZONE"

gcloud dns record-sets transaction add "$INSTANCE_PUBLIC_IP" \
  --name="*.$GCLOUD_DNS_DOMAIN." \
  --type=A \
  --ttl=60 \
  --zone="$GCLOUD_DNS_ZONE"

gcloud dns record-sets transaction execute --zone="$GCLOUD_DNS_ZONE"
```

### Wait for DNS to propagate

```sh
dig +short "test.$GCLOUD_DNS_DOMAIN"
```

## Clean up

```sh
gcloud compute instances delete p1
gcloud compute firewall-rules delete k3s
gcloud compute firewall-rules delete inbound-http

gcloud dns record-sets transaction start --zone="$GCLOUD_DNS_ZONE"

gcloud dns record-sets list                \
  --filter=name="$GCLOUD_DNS_DOMAIN."      \
  --filter=type=A                          \
  --zone="$GCLOUD_DNS_ZONE"                \
  --format="get[separator=|](name,DATA)" | \
  while read -r line; do
    gcloud dns record-sets transaction remove "${line##*|}" \
      --zone="$GCLOUD_DNS_ZONE"                             \
      --type=A                                              \
      --name="${line%%|*}"                                  \
      --ttl=60
done

gcloud dns record-sets transaction execute --zone="$GCLOUD_DNS_ZONE"
```
