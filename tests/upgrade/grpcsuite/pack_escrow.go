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
	dep := createDeploymentFromSDL(s, s.TenantSigner())

	// The deployment query returns its escrow account (incl. its ID) directly.
	depResp, err := dvbeta.NewQueryClient(s.Conn).Deployment(s.Ctx, &dvbeta.QueryDeploymentRequest{ID: dep})
	require.NoError(s.T, err, "Deployment (for escrow account id)")
	accountID := depResp.EscrowAccount.ID

	// The deployment's escrow account is uact-denominated (the deployment deposit is
	// uact), so the additional deposit must also be uact; the tenant (funder) holds it.
	depositCoin := sdk.NewCoin(sdkutil.DenomUact, sdkmath.NewInt(1_000_000))

	// MsgAccountDeposit — the signer need not be the account owner.
	s.BroadcastOK(s.TenantSigner(), &ev1.MsgAccountDeposit{
		Signer:  s.TenantAddr().String(),
		ID:      accountID,
		Deposit: depositv1.Deposit{Amount: depositCoin, Sources: depositv1.Sources{depositv1.SourceBalance}},
	})

	q := ev1.NewQueryClient(s.Conn)

	// Accounts: the deposit above proved the specific account exists; assert the
	// account set is non-empty and that the pagination Limit caps results.
	accts, err := q.Accounts(s.Ctx, &ev1.QueryAccountsRequest{Pagination: &sdkquery.PageRequest{Limit: 100}})
	require.NoError(s.T, err, "escrow Accounts")
	require.NotEmpty(s.T, accts.Accounts, "there should be escrow accounts")
	paged, err := q.Accounts(s.Ctx, &ev1.QueryAccountsRequest{Pagination: &sdkquery.PageRequest{Limit: 1}})
	require.NoError(s.T, err, "escrow Accounts (paginated)")
	require.LessOrEqual(s.T, len(paged.Accounts), 1, "pagination Limit=1 must cap results")

	// Payments (paginated).
	_, err = q.Payments(s.Ctx, &ev1.QueryPaymentsRequest{Pagination: &sdkquery.PageRequest{Limit: 10}})
	require.NoError(s.T, err, "escrow Payments")

	// Negative: deposit to an account id that does not exist.
	bogus := accountID
	bogus.XID = "akash1nonexistentnonexistentnonexistentnn/999999999"
	s.BroadcastExpectErr(s.TenantSigner(), &ev1.MsgAccountDeposit{
		Signer:  s.TenantAddr().String(),
		ID:      bogus,
		Deposit: depositv1.Deposit{Amount: depositCoin, Sources: depositv1.Sources{depositv1.SourceBalance}},
	})

	s.logf("escrow deposit complete (account xid %s)", accountID.XID)
}
