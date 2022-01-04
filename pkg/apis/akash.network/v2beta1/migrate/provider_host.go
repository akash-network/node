package migrate

import (
	v1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	"github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1"
)

func ProviderHostsSpecFromV1(from v1.ProviderHostSpec) v2beta1.ProviderHostSpec {
	to := v2beta1.ProviderHostSpec{
		Owner:        from.Owner,
		Provider:     from.Provider,
		Hostname:     from.Hostname,
		Dseq:         from.Dseq,
		Gseq:         from.Gseq,
		Oseq:         from.Oseq,
		ServiceName:  from.ServiceName,
		ExternalPort: from.ExternalPort,
	}

	return to
}
