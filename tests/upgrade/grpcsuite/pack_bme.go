package grpcsuite

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	bmev1 "pkg.akt.dev/go/node/bme/v1"
	"pkg.akt.dev/go/sdkutil"
)

// bmePack exercises the burn/mint engine. It depends on the oracle pack having fed
// an AKT price (bme converts via oracle prices) and runs before gov-params.
type bmePack struct{}

func (bmePack) Name() string { return "bme" }

func (bmePack) Available(d *Discovery) bool { return d.HasModule("akash.bme.v1") }

func (bmePack) Run(s *Suite) {
	q := bmev1.NewQueryClient(s.Conn)
	actor := s.FundAccountDefault("bme") // holds both uakt and uact

	akt := func(n int64) sdk.Coin { return sdk.NewCoin(sdkutil.DenomUakt, sdkmath.NewInt(n)) }
	act := func(n int64) sdk.Coin { return sdk.NewCoin(sdkutil.DenomUact, sdkmath.NewInt(n)) }

	// Seed the vault with AKT from a funded account (gov-gated).
	s.PassGovProposal("grpcsuite: fund bme vault",
		&bmev1.MsgFundVault{Authority: s.GovAuthority(), Amount: akt(10_000_000), Source: actor.String()})

	// Re-feed a fresh AKT price: the gov proposal above consumed several seconds and
	// bme refuses to queue conversions when the oracle price is not healthy.
	s.feedAKTPrice(3)

	// Mint/burn acceptance depends on the live collateral ratio and the circuit
	// breaker, which are not deterministically controllable here. Each message is
	// exercised over gRPC and must either execute or be correctly rejected by the
	// circuit breaker (only minting ACT is typically enabled on main).
	tolerate := []string{"circuit breaker", "collateral", "insufficient"}
	s.BroadcastTolerant("bme", tolerate, &bmev1.MsgMintACT{Owner: actor.String(), To: actor.String(), CoinsToBurn: akt(1_000_000)})
	s.BroadcastTolerant("bme", tolerate, &bmev1.MsgBurnACT{Owner: actor.String(), To: actor.String(), CoinsToBurn: act(1_000_000)})
	s.BroadcastTolerant("bme", tolerate, &bmev1.MsgBurnMint{Owner: actor.String(), To: actor.String(), CoinsToBurn: akt(1_000_000), DenomToMint: sdkutil.DenomUact})

	var err error

	_, err = q.VaultState(s.Ctx, &bmev1.QueryVaultStateRequest{})
	require.NoError(s.T, err, "VaultState")
	_, err = q.Status(s.Ctx, &bmev1.QueryStatusRequest{})
	require.NoError(s.T, err, "Status")
	s.logf("bme complete (fund vault + mint ACT + burn ACT + burn/mint)")
}
