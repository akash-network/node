package testutil

import (
	"testing"

	atypes "github.com/akash-network/akash-api/go/node/audit/v1beta3"
)

func AuditedProvider(t testing.TB) (atypes.ProviderID, atypes.Provider) {
	t.Helper()

	id := atypes.ProviderID{
		Auditor: AccAddress(t),
		Owner:   AccAddress(t),
	}

	return id, atypes.Provider{
		Auditor:    id.Auditor.String(),
		Owner:      id.Owner.String(),
		Attributes: Attributes(t),
	}
}
