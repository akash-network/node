package testutil

import (
	"testing"

	ptypes "github.com/akash-network/node/x/provider/types/v1beta2"
)

func Provider(t testing.TB) ptypes.Provider {
	t.Helper()

	return ptypes.Provider{
		Owner:      AccAddress(t).String(),
		HostURI:    Hostname(t),
		Attributes: Attributes(t),
		Info: ptypes.ProviderInfo{
			EMail:   "test@example.com",
			Website: ProviderHostname(t),
		},
	}
}
