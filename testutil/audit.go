package testutil

import (
	"testing"

	atypes "github.com/ovrclk/akash/x/audit/types"
)

func AuditedProvider(t testing.TB) (atypes.ProviderID, atypes.Provider) {
	t.Helper()

	id := atypes.ProviderID{
		Validator: AccAddress(t),
		Owner:     AccAddress(t),
	}

	return id, atypes.Provider{
		Validator:  id.Validator.String(),
		Owner:      id.Owner.String(),
		Attributes: Attributes(t),
	}
}
