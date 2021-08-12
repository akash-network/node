package builder

// nolint:deadcode,golint

import (
	"fmt"
	"strconv"

	"github.com/tendermint/tendermint/libs/log"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/intstr"

	manifesttypes "github.com/ovrclk/akash/manifest"
	clusterUtil "github.com/ovrclk/akash/provider/cluster/util"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

const (
	AkashManagedLabelName         = "akash.network"
	AkashManifestServiceLabelName = "akash.network/manifest-service"
	AkashNetworkStorageClasses    = "akash.network/storageclasses"

	akashNetworkNamespace = "akash.network/namespace"

	AkashLeaseOwnerLabelName    = "akash.network/lease.id.owner"
	AkashLeaseDSeqLabelName     = "akash.network/lease.id.dseq"
	AkashLeaseGSeqLabelName     = "akash.network/lease.id.gseq"
	AkashLeaseOSeqLabelName     = "akash.network/lease.id.oseq"
	AkashLeaseProviderLabelName = "akash.network/lease.id.provider"

	akashDeploymentPolicyName = "akash-deployment-restrictions"
)

const runtimeClassNoneValue = "none"

const (
	envVarAkashGroupSequence         = "AKASH_GROUP_SEQUENCE"
	envVarAkashDeploymentSequence    = "AKASH_DEPLOYMENT_SEQUENCE"
	envVarAkashOrderSequence         = "AKASH_ORDER_SEQUENCE"
	envVarAkashOwner                 = "AKASH_OWNER"
	envVarAkashProvider              = "AKASH_PROVIDER"
	envVarAkashClusterPublicHostname = "AKASH_CLUSTER_PUBLIC_HOSTNAME"
)

var (
	dnsPort     = intstr.FromInt(53)
	dnsProtocol = corev1.Protocol("UDP")
)

type builderBase interface {
	NS() string
	Name() string
}

type builder struct {
	log      log.Logger
	settings Settings
	lid      mtypes.LeaseID
	group    *manifesttypes.Group
}

var _ builderBase = (*builder)(nil)

func (b *builder) NS() string {
	return LidNS(b.lid)
}

func (b *builder) Name() string {
	return b.NS()
}

func (b *builder) labels() map[string]string {
	return map[string]string{
		AkashManagedLabelName: "true",
		akashNetworkNamespace: LidNS(b.lid),
	}
}

func addIfNotPresent(envVarsAlreadyAdded map[string]int, env []corev1.EnvVar, key string, value interface{}) []corev1.EnvVar {
	_, exists := envVarsAlreadyAdded[key]
	if exists {
		return env
	}

	env = append(env, corev1.EnvVar{Name: key, Value: fmt.Sprintf("%v", value)})
	return env
}

const SuffixForNodePortServiceName = "-np"

func makeGlobalServiceNameFromBasename(basename string) string {
	return fmt.Sprintf("%s%s", basename, SuffixForNodePortServiceName)
}

// LidNS generates a unique sha256 sum for identifying a provider's object name.
func LidNS(lid mtypes.LeaseID) string {
	return clusterUtil.LeaseIDToNamespace(lid)
}

func AppendLeaseLabels(lid mtypes.LeaseID, labels map[string]string) map[string]string {
	labels[AkashLeaseOwnerLabelName] = lid.Owner
	labels[AkashLeaseDSeqLabelName] = strconv.FormatUint(lid.DSeq, 10)
	labels[AkashLeaseGSeqLabelName] = strconv.FormatUint(uint64(lid.GSeq), 10)
	labels[AkashLeaseOSeqLabelName] = strconv.FormatUint(uint64(lid.OSeq), 10)
	labels[AkashLeaseProviderLabelName] = lid.Provider
	return labels
}
