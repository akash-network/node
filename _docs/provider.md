Provider Design Document Placeholder

Config

```
ClusterWaitReadyDuration time.Duration `env:"AKASH_CLUSTER_WAIT_READY_DURATION" envDefault:"5s"`
InventoryResourcePollPeriod     time.Duration `env:"AKASH_INVENTORY_RESOURCE_POLL_PERIOD" envDefault:"5s"`
InventoryResourceDebugFrequency uint          `env:"AKASH_INVENTORY_RESOURCE_DEBUG_FREQUENCY" envDefault:"10"`
// gcp:    NodePort
// others: ClusterIP
DeploymentServiceType corev1.ServiceType `env:"AKASH_DEPLOYMENT_SERVICE_TYPE" envDefault:"NodePort"`

// gcp:    False
// others: true
DeploymentIngressStaticHosts bool `env:"AKASH_DEPLOYMENT_INGRESS_STATIC_HOSTS" envDefault:"false"`

DeploymentIngressExposeLBHosts bool `env:"AKASH_DEPLOYMENT_INGRESS_EXPOSE_LB_HOSTS" envDefault:"true"`

ManifestLingerDuration time.Duration `env:"AKASH_MANIFEST_LINGER_DURATION" envDefault:"5m"`

cmd.Flags().String("private-key", "", "import private key")
cmd.Flags().Bool("kube", false, "use kubernetes cluster")
cmd.Flags().String("manifest-ns", "lease", "set manifest namespace")
```
