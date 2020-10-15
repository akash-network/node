package testutil

import (
	"testing"

	ptypes "github.com/ovrclk/akash/x/provider/types"
)

func Provider(t testing.TB) ptypes.Provider {
	t.Helper()

	return ptypes.Provider{
		Owner:      AccAddress(t).String(),
		HostURI:    Hostname(t),
		Attributes: Attributes(t),
	}
}
