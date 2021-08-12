package builder

// nolint:deadcode,golint

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strconv"
	"strings"

	"github.com/tendermint/tendermint/libs/log"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/intstr"

	manifesttypes "github.com/ovrclk/akash/manifest"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

const (
	AkashManagedLabelName         = "akash.network"
	AkashManifestServiceLabelName = "akash.network/manifest-service"
	AkashNetworkStorageClasses    = "akash.network/storageclasses"

	akashNetworkNamespace = "akash.network/namespace"

	akashLeaseOwnerLabelName    = "akash.network/lease.id.owner"
	akashLeaseDSeqLabelName     = "akash.network/lease.id.dseq"
	akashLeaseGSeqLabelName     = "akash.network/lease.id.gseq"
	akashLeaseOSeqLabelName     = "akash.network/lease.id.oseq"
	akashLeaseProviderLabelName = "akash.network/lease.id.provider"

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
	path := lid.String()
	// DNS-1123 label must consist of lower case alphanumeric characters or '-',
	// and must start and end with an alphanumeric character
	// (e.g. 'my-name',  or '123-abc', regex used for validation
	// is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')
	sha := sha256.Sum224([]byte(path))
	return strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(sha[:]))
}

func appendLeaseLabels(lid mtypes.LeaseID, labels map[string]string) map[string]string {
	labels[akashLeaseOwnerLabelName] = lid.Owner
	labels[akashLeaseDSeqLabelName] = strconv.FormatUint(lid.DSeq, 10)
	labels[akashLeaseGSeqLabelName] = strconv.FormatUint(uint64(lid.GSeq), 10)
	labels[akashLeaseOSeqLabelName] = strconv.FormatUint(uint64(lid.OSeq), 10)
	labels[akashLeaseProviderLabelName] = lid.Provider
	return labels
}
