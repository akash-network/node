package testutil

import (
	"testing"

	atypes "pkg.akt.dev/go/node/audit/v1"
)

func AuditedProvider(t testing.TB) (atypes.ProviderID, atypes.AuditedProvider) {
	t.Helper()

	id := atypes.ProviderID{
		Auditor: AccAddress(t),
		Owner:   AccAddress(t),
	}

	return id, atypes.AuditedProvider{
		Auditor:    id.Auditor.String(),
		Owner:      id.Owner.String(),
		Attributes: Attributes(t),
	}
}
