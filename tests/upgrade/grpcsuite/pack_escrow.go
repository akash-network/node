package grpcsuite

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	ev1 "pkg.akt.dev/go/node/escrow/v1"
	depositv1 "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/sdkutil"
)

type escrowPack struct{}

func (escrowPack) Name() string { return "escrow" }

func (escrowPack) Available(d *Discovery) bool { return d.HasModule("akash.escrow.v1") }

func (escrowPack) Run(s *Suite) {
	// Fresh deployment so the escrow account is open and independent of other packs.
	dep := createDeploymentFromSDL(s, "tenant")

	// The deployment query returns its escrow account (incl. its ID) directly.
	depResp, err := dvbeta.NewQueryClient(s.Conn).Deployment(s.Ctx, &dvbeta.QueryDeploymentRequest{ID: dep})
	require.NoError(s.T, err, "Deployment (for escrow account id)")
	accountID := depResp.EscrowAccount.ID

	// MsgAccountDeposit — the signer need not be the account owner.
	s.BroadcastOK("tenant", &ev1.MsgAccountDeposit{
		Signer:  s.Addr("tenant").String(),
		ID:      accountID,
		Deposit: depositv1.Deposit{Amount: sdk.NewCoin(sdkutil.DenomUact, sdkmath.NewInt(1_000_000)), Sources: depositv1.Sources{depositv1.SourceBalance}},
	})

	q := ev1.NewQueryClient(s.Conn)
	_, err = q.Accounts(s.Ctx, &ev1.QueryAccountsRequest{Pagination: &sdkquery.PageRequest{Limit: 10}})
	require.NoError(s.T, err, "escrow Accounts")
	_, err = q.Payments(s.Ctx, &ev1.QueryPaymentsRequest{Pagination: &sdkquery.PageRequest{Limit: 10}})
	require.NoError(s.T, err, "escrow Payments")
	s.logf("escrow deposit complete (account xid %s)", accountID.XID)
}
