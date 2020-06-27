# Kubernetes cluster on GCP

Follow these steps to create a Kubernetes cluster on
[GCP](https://cloud.google.com/) using [`gcloud`](https://cloud.google.com/sdk/gcloud).

The K3S portions of this are based off of [this](https://starkandwayne.com/blog/trying-tiny-k3s-on-google-cloud-with-k3sup/)
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

```sh
gcloud config set project "$GCLOUD_PROJECT"
```

```sh
gcloud compute project-info add-metadata \
    --metadata google-compute-default-region=europe-west1,google-compute-default-zone=europe-west1-b
gcloud init
```

### Create Instance

```sh
gcloud compute instances create p1 \
    --machine-type n1-standard-1 \
    --tags k3s,k3s-master
```

### Download `k3sup`

```sh
(cd bin && curl -sLS https://get.k3sup.dev | sh )
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
./bin/k3sup install --ip "$INSTANCE_PUBLIC_IP" --context k3s --ssh-key ~/.ssh/google_compute_engine --user "$(whoami)"
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
