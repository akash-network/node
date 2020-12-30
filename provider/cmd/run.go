package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ovrclk/akash/client/broadcaster"
	"github.com/ovrclk/akash/provider/bidengine"

	cosmosclient "github.com/cosmos/cosmos-sdk/client"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/go-kit/kit/log/term"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/libs/log"
	"golang.org/x/sync/errgroup"

	"github.com/ovrclk/akash/client"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/events"
	"github.com/ovrclk/akash/provider"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/kube"
	"github.com/ovrclk/akash/provider/gateway"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	amodule "github.com/ovrclk/akash/x/audit"
	dmodule "github.com/ovrclk/akash/x/deployment"
	mmodule "github.com/ovrclk/akash/x/market"
	pmodule "github.com/ovrclk/akash/x/provider"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

const (
	// FlagClusterK8s informs the provider to scan and utilize localized kubernetes client configuration
	FlagClusterK8s = "cluster-k8s"
	// FlagK8sManifestNS
	FlagK8sManifestNS = "k8s-manifest-ns"
	// FlagGatewayListenAddress determines listening address for Manifests
	FlagGatewayListenAddress             = "gateway-listen-address"
	FlagBidPricingStrategy               = "bid-price-strategy"
	FlagBidPriceCPUScale                 = "bid-price-cpu-scale"
	FlagBidPriceMemoryScale              = "bid-price-memory-scale"
	FlagBidPriceStorageScale             = "bid-price-storage-scale"
	FlagBidPriceEndpointScale            = "bid-price-endpoint-scale"
	FlagBidPriceScriptPath               = "bid-price-script-path"
	FlagBidPriceScriptProcessLimit       = "bid-price-script-process-limit"
	FlagBidPriceScriptTimeout            = "bid-price-script-process-timeout"
	FlagClusterPublicHostname            = "cluster-public-hostname"
	FlagClusterNodePortQuantity          = "cluster-node-port-quantity"
	FlagClusterWaitReadyDuration         = "cluster-wait-ready-duration"
	FlagInventoryResourcePollPeriod      = "inventory-resource-poll-period"
	FlagInventoryResourceDebugFrequency  = "inventory-resource-debug-frequency"
	FlagDeploymentIngressStaticHosts     = "deployment-ingress-static-hosts"
	FlagDeploymentIngressDomain          = "deployment-ingress-domain"
	FlagDeploymentIngressExposeLBHosts   = "deployment-ingress-expose-lb-hosts"
	FlagDeploymentNetworkPoliciesEnabled = "deployment-network-policies-enabled"
	FlagOvercommitPercentMemory          = "overcommit-pct-mem"
	FlagOvercommitPercentCPU             = "overcommit-pct-cpu"
	FlagOvercommitPercentStorage         = "overcommit-pct-storage"
)

var (
	errInvalidConfig = errors.New("Invalid configuration")
)

// RunCmd launches the Akash Provider service
func RunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "run akash provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			return common.RunForever(func(ctx context.Context) error {
				return doRunCmd(ctx, cmd, args)
			})
		},
	}

	cmd.Flags().String(flags.FlagChainID, "", "The network chain ID")
	if err := viper.BindPFlag(flags.FlagChainID, cmd.Flags().Lookup(flags.FlagChainID)); err != nil {
		return nil
	}

	flags.AddTxFlagsToCmd(cmd)

	cmd.Flags().Bool(FlagClusterK8s, false, "Use Kubernetes cluster")
	if err := viper.BindPFlag(FlagClusterK8s, cmd.Flags().Lookup(FlagClusterK8s)); err != nil {
		return nil
	}

	cmd.Flags().String(FlagK8sManifestNS, "lease", "Cluster manifest namespace")
	if err := viper.BindPFlag(FlagK8sManifestNS, cmd.Flags().Lookup(FlagK8sManifestNS)); err != nil {
		return nil
	}

	cmd.Flags().String(FlagGatewayListenAddress, "0.0.0.0:8080", "Gateway listen address")
	if err := viper.BindPFlag(FlagGatewayListenAddress, cmd.Flags().Lookup(FlagGatewayListenAddress)); err != nil {
		return nil
	}

	cmd.Flags().String(FlagBidPricingStrategy, "scale", "Pricing strategy to use")
	if err := viper.BindPFlag(FlagBidPricingStrategy, cmd.Flags().Lookup(FlagBidPricingStrategy)); err != nil {
		return nil
	}

	cmd.Flags().Uint64(FlagBidPriceCPUScale, 0, "cpu pricing scale in uakt per millicpu")
	if err := viper.BindPFlag(FlagBidPriceCPUScale, cmd.Flags().Lookup(FlagBidPriceCPUScale)); err != nil {
		return nil
	}

	cmd.Flags().Uint64(FlagBidPriceMemoryScale, 0, "memory pricing scale in uakt per megabyte")
	if err := viper.BindPFlag(FlagBidPriceMemoryScale, cmd.Flags().Lookup(FlagBidPriceMemoryScale)); err != nil {
		return nil
	}

	cmd.Flags().Uint64(FlagBidPriceStorageScale, 0, "storage pricing scale in uakt per megabyte")
	if err := viper.BindPFlag(FlagBidPriceStorageScale, cmd.Flags().Lookup(FlagBidPriceStorageScale)); err != nil {
		return nil
	}

	cmd.Flags().Uint64(FlagBidPriceEndpointScale, 0, "endpoint pricing scale in uakt")
	if err := viper.BindPFlag(FlagBidPriceEndpointScale, cmd.Flags().Lookup(FlagBidPriceEndpointScale)); err != nil {
		return nil
	}

	cmd.Flags().String(FlagBidPriceScriptPath, "", "path to script to run for computing bid price")
	if err := viper.BindPFlag(FlagBidPriceScriptPath, cmd.Flags().Lookup(FlagBidPriceScriptPath)); err != nil {
		return nil
	}

	cmd.Flags().Uint(FlagBidPriceScriptProcessLimit, 32, "limit to the number of scripts run concurrently for bid pricing")
	if err := viper.BindPFlag(FlagBidPriceScriptProcessLimit, cmd.Flags().Lookup(FlagBidPriceScriptProcessLimit)); err != nil {
		return nil
	}

	cmd.Flags().Duration(FlagBidPriceScriptTimeout, time.Second*10, "execution timelimit for bid pricing as a duration")
	if err := viper.BindPFlag(FlagBidPriceScriptTimeout, cmd.Flags().Lookup(FlagBidPriceScriptTimeout)); err != nil {
		return nil
	}

	cmd.Flags().String(FlagClusterPublicHostname, "", "The public IP of the Kubernetes cluster")
	if err := viper.BindPFlag(FlagClusterPublicHostname, cmd.Flags().Lookup(FlagClusterPublicHostname)); err != nil {
		return nil
	}

	cmd.Flags().Uint(FlagClusterNodePortQuantity, 1, "The number of node ports available on the Kubernetes cluster")
	if err := viper.BindPFlag(FlagClusterNodePortQuantity, cmd.Flags().Lookup(FlagClusterNodePortQuantity)); err != nil {
		return nil
	}

	cmd.Flags().Duration(FlagClusterWaitReadyDuration, time.Second*5, "The time to wait for the cluster to be available")
	if err := viper.BindPFlag(FlagClusterWaitReadyDuration, cmd.Flags().Lookup(FlagClusterWaitReadyDuration)); err != nil {
		return nil
	}

	cmd.Flags().Duration(FlagInventoryResourcePollPeriod, time.Second*5, "The period to poll the cluster inventory")
	if err := viper.BindPFlag(FlagInventoryResourcePollPeriod, cmd.Flags().Lookup(FlagInventoryResourcePollPeriod)); err != nil {
		return nil
	}

	cmd.Flags().Uint(FlagInventoryResourceDebugFrequency, 10, "The rate at which to log all inventory resources")
	if err := viper.BindPFlag(FlagInventoryResourceDebugFrequency, cmd.Flags().Lookup(FlagInventoryResourceDebugFrequency)); err != nil {
		return nil
	}

	cmd.Flags().Bool(FlagDeploymentIngressStaticHosts, false, "")
	if err := viper.BindPFlag(FlagDeploymentIngressStaticHosts, cmd.Flags().Lookup(FlagDeploymentIngressStaticHosts)); err != nil {
		return nil
	}

	cmd.Flags().String(FlagDeploymentIngressDomain, "", "")
	if err := viper.BindPFlag(FlagDeploymentIngressDomain, cmd.Flags().Lookup(FlagDeploymentIngressDomain)); err != nil {
		return nil
	}

	cmd.Flags().Bool(FlagDeploymentIngressExposeLBHosts, false, "")
	if err := viper.BindPFlag(FlagDeploymentIngressExposeLBHosts, cmd.Flags().Lookup(FlagDeploymentIngressExposeLBHosts)); err != nil {
		return nil
	}

	cmd.Flags().Bool(FlagDeploymentNetworkPoliciesEnabled, false, "Enable network policies")
	if err := viper.BindPFlag(FlagDeploymentNetworkPoliciesEnabled, cmd.Flags().Lookup(FlagDeploymentNetworkPoliciesEnabled)); err != nil {
		return nil
	}

	cmd.Flags().Uint64(FlagOvercommitPercentMemory, 0, "Percentage of memory overcommit")
	if err := viper.BindPFlag(FlagOvercommitPercentMemory, cmd.Flags().Lookup(FlagOvercommitPercentMemory)); err != nil {
		return nil
	}

	cmd.Flags().Uint64(FlagOvercommitPercentCPU, 0, "Percentage of CPU overcommit")
	if err := viper.BindPFlag(FlagOvercommitPercentCPU, cmd.Flags().Lookup(FlagOvercommitPercentCPU)); err != nil {
		return nil
	}

	cmd.Flags().Uint64(FlagOvercommitPercentStorage, 0, "Percentage of storage overcommit")
	if err := viper.BindPFlag(FlagOvercommitPercentStorage, cmd.Flags().Lookup(FlagOvercommitPercentStorage)); err != nil {
		return nil
	}

	return cmd
}

const (
	bidPricingStrategyScale       = "scale"
	bidPricingStrategyRandomRange = "randomRange"
	bidPricingStrategyShellScript = "shellScript"
)

var allowedBidPricingStrategies = [...]string{
	bidPricingStrategyScale,
	bidPricingStrategyRandomRange,
	bidPricingStrategyShellScript,
}

var errNoSuchBidPricingStrategy = fmt.Errorf("No such bid pricing strategy. Allowed: %v", allowedBidPricingStrategies)

func createBidPricingStrategy(strategy string) (bidengine.BidPricingStrategy, error) {
	if strategy == bidPricingStrategyScale {
		cpuScale := viper.GetUint64(FlagBidPriceCPUScale)
		memoryScale := viper.GetUint64(FlagBidPriceMemoryScale)
		storageScale := viper.GetUint64(FlagBidPriceStorageScale)
		endpointScale := viper.GetUint64(FlagBidPriceEndpointScale)

		return bidengine.MakeScalePricing(cpuScale, memoryScale, storageScale, endpointScale)
	}

	if strategy == bidPricingStrategyRandomRange {
		return bidengine.MakeRandomRangePricing()
	}

	if strategy == bidPricingStrategyShellScript {
		scriptPath := viper.GetString(FlagBidPriceScriptPath)
		processLimit := viper.GetUint(FlagBidPriceScriptProcessLimit)
		runtimeLimit := viper.GetDuration(FlagBidPriceScriptTimeout)
		return bidengine.MakeShellScriptPricing(scriptPath, processLimit, runtimeLimit)
	}

	return nil, errNoSuchBidPricingStrategy
}

// doRunCmd initializes all of the Provider functionality, hangs, and awaits shutdown signals.
func doRunCmd(ctx context.Context, cmd *cobra.Command, _ []string) error {
	clusterPublicHostname := viper.GetString(FlagClusterPublicHostname)
	// TODO - validate that clusterPublicHostname is a valid hostname
	nodePortQuantity := viper.GetUint(FlagClusterNodePortQuantity)
	clusterWaitReadyDuration := viper.GetDuration(FlagClusterWaitReadyDuration)
	inventoryResourcePollPeriod := viper.GetDuration(FlagInventoryResourcePollPeriod)
	inventoryResourceDebugFreq := viper.GetUint(FlagInventoryResourceDebugFrequency)
	deploymentIngressStaticHosts := viper.GetBool(FlagDeploymentIngressStaticHosts)
	deploymentIngressDomain := viper.GetString(FlagDeploymentIngressDomain)
	deploymentNetworkPoliciesEnabled := viper.GetBool(FlagDeploymentNetworkPoliciesEnabled)
	strategy := viper.GetString(FlagBidPricingStrategy)
	deploymentIngressExposeLBHosts := viper.GetBool(FlagDeploymentIngressExposeLBHosts)
	from := viper.GetString(flags.FlagFrom)
	overcommitPercentStorage := 1.0 + float64(viper.GetUint64(FlagOvercommitPercentStorage)/100.0)
	overcommitPercentCPU := 1.0 + float64(viper.GetUint64(FlagOvercommitPercentCPU)/100.0)
	overcommitPercentMemory := 1.0 + float64(viper.GetUint64(FlagOvercommitPercentMemory)/100.0)
	pricing, err := createBidPricingStrategy(strategy)

	if err != nil {
		return err
	}

	cctx := sdkclient.GetClientContextFromCmd(cmd)

	_, _, err = cosmosclient.GetFromFields(cctx.Keyring, from, false)
	if err != nil {
		return err
	}

	cctx, err = sdkclient.ReadTxCommandFlags(cctx, cmd.Flags())
	if err != nil {
		return err
	}

	txFactory := tx.NewFactoryCLI(cctx, cmd.Flags()).WithTxConfig(cctx.TxConfig).WithAccountRetriever(cctx.AccountRetriever)

	keyname := cctx.GetFromName()
	info, err := txFactory.Keybase().Key(keyname)
	if err != nil {
		return err
	}

	gwaddr := viper.GetString(FlagGatewayListenAddress)

	log := openLogger()

	broadcaster, err := broadcaster.NewSerialClient(log, cctx, txFactory, info)
	if err != nil {
		return err
	}

	// TODO: actually get the passphrase?
	// passphrase, err := keys.GetPassphrase(fromName)
	aclient := client.NewClientWithBroadcaster(
		log,
		cctx,
		txFactory,
		info,
		client.NewQueryClient(
			dmodule.AppModuleBasic{}.GetQueryClient(cctx),
			mmodule.AppModuleBasic{}.GetQueryClient(cctx),
			pmodule.AppModuleBasic{}.GetQueryClient(cctx),
			amodule.AppModuleBasic{}.GetQueryClient(cctx),
		),
		broadcaster,
	)

	res, err := aclient.Query().Provider(
		context.Background(),
		&ptypes.QueryProviderRequest{Owner: info.GetAddress().String()},
	)
	if err != nil {
		return err
	}

	pinfo := &res.Provider

	// k8s client creation
	kubeSettings := kube.NewDefaultSettings()
	kubeSettings.DeploymentIngressDomain = deploymentIngressDomain
	kubeSettings.DeploymentIngressExposeLBHosts = deploymentIngressExposeLBHosts
	kubeSettings.DeploymentIngressStaticHosts = deploymentIngressStaticHosts
	kubeSettings.NetworkPoliciesEnabled = deploymentNetworkPoliciesEnabled
	kubeSettings.ClusterPublicHostname = clusterPublicHostname
	kubeSettings.CPUCommitLevel = overcommitPercentCPU
	kubeSettings.MemoryCommitLevel = overcommitPercentMemory
	kubeSettings.StorageCommitLevel = overcommitPercentStorage

	cclient, err := createClusterClient(log, cmd, kubeSettings)
	if err != nil {
		return err
	}

	session := session.New(log, aclient, pinfo)

	if err := cctx.Client.Start(); err != nil {
		return err
	}

	bus := pubsub.NewBus()
	defer bus.Close()

	group, ctx := errgroup.WithContext(ctx)

	// Provider service creation
	config := provider.NewDefaultConfig()
	config.ClusterWaitReadyDuration = clusterWaitReadyDuration
	config.ClusterPublicHostname = clusterPublicHostname
	config.ClusterExternalPortQuantity = nodePortQuantity
	config.InventoryResourceDebugFrequency = inventoryResourceDebugFreq
	config.InventoryResourcePollPeriod = inventoryResourcePollPeriod
	config.CPUCommitLevel = overcommitPercentCPU
	config.MemoryCommitLevel = overcommitPercentMemory
	config.StorageCommitLevel = overcommitPercentStorage

	config.BPS = pricing
	service, err := provider.NewService(ctx, session, bus, cclient, config)

	if err != nil {
		return group.Wait()
	}

	gateway := gateway.NewServer(ctx, log, service, gwaddr)

	group.Go(func() error {
		return events.Publish(ctx, cctx.Client, "provider-cli", bus)
	})

	group.Go(func() error {
		<-service.Done()
		return nil
	})

	group.Go(gateway.ListenAndServe)

	group.Go(func() error {
		<-ctx.Done()
		return gateway.Close()
	})

	err = group.Wait()
	broadcaster.Close()
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func openLogger() log.Logger {
	// logger with no color output - current debug colors are invisible for me.
	return log.NewTMLoggerWithColorFn(log.NewSyncWriter(os.Stdout), func(_ ...interface{}) term.FgBgColor {
		return term.FgBgColor{}
	})
}

func createClusterClient(log log.Logger, _ *cobra.Command, settings kube.Settings) (cluster.Client, error) {
	if !viper.GetBool(FlagClusterK8s) {
		// Condition that there is no Kubernetes API to work with.
		return cluster.NullClient(), nil
	}
	ns := viper.GetString(FlagK8sManifestNS)
	if ns == "" {
		return nil, fmt.Errorf("%w: --%s required", errInvalidConfig, FlagK8sManifestNS)
	}
	return kube.NewClient(log, ns, settings)
}

func showErrorToUser(err error) error {
	// If the error has a complete message associated with it then show it
	clientResponseError, ok := err.(gateway.ClientResponseError)
	if ok && 0 != len(clientResponseError.Message) {
		fmt.Fprintf(os.Stderr, "provider error messsage:\n%v\n", clientResponseError.Message)
	}

	return err
}
