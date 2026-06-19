package grpcsuite

import (
	"context"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"

	bmev1 "pkg.akt.dev/go/node/bme/v1"
	oraclev2 "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/sdkutil"
)

const (
	funderUactTarget = int64(100_000_000)
	bmeVaultSeedUakt = int64(100_000_000)
	bmeMintBurnUakt  = int64(50_000_000)
)

func (s *Suite) bootstrapFunderUact() {
	s.T.Helper()

	if !s.Disc.HasModule("akash.deployment.v1beta4") && !s.Disc.HasModule("akash.escrow.v1") {
		return
	}

	target := sdk.NewInt64Coin(sdkutil.DenomUact, funderUactTarget)
	current := s.balanceOf(s.Env.FunderAddr, sdkutil.DenomUact)
	if current.Amount.GTE(target.Amount) {
		s.logf("funder already has %s; uact bootstrap skipped", current)
		return
	}

	require.True(s.T, s.Disc.HasModule("akash.oracle.v2"), "grpcsuite: oracle is required to mint uact")
	require.True(s.T, s.Disc.HasModule("akash.bme.v1"), "grpcsuite: bme is required to mint uact")

	s.logf("funder has %s; minting at least %s before deposit-bearing packs", current, target)
	s.authorizeFunderOracleAndFundBMEVault()
	s.feedAKTPriceAs(s.Env.Funder, s.Env.FunderAddr, 3)
	s.waitBMEMintsAllowed()

	for attempt := 1; attempt <= 3; attempt++ {
		current = s.balanceOf(s.Env.FunderAddr, sdkutil.DenomUact)
		if current.Amount.GTE(target.Amount) {
			s.logf("funder uact bootstrap complete: %s", current)
			return
		}

		s.feedAKTPriceAs(s.Env.Funder, s.Env.FunderAddr, 3)
		s.BroadcastOK(s.Env.Funder, &bmev1.MsgMintACT{
			Owner:       s.Env.FunderAddr.String(),
			To:          s.Env.FunderAddr.String(),
			CoinsToBurn: sdk.NewCoin(sdkutil.DenomUakt, sdkmath.NewInt(bmeMintBurnUakt)),
		})
		if s.waitForFunderUact(target, 2*time.Minute) {
			return
		}
		s.logf("funder uact still below target after mint attempt %d: %s", attempt, s.balanceOf(s.Env.FunderAddr, sdkutil.DenomUact))
	}

	final := s.balanceOf(s.Env.FunderAddr, sdkutil.DenomUact)
	require.Truef(s.T, final.Amount.GTE(target.Amount), "grpcsuite: funder uact bootstrap ended with %s, need at least %s", final, target)
}

func (s *Suite) authorizeFunderOracleAndFundBMEVault() {
	s.T.Helper()

	authority := s.GovAuthority()
	var msgs []sdk.Msg

	q := oraclev2.NewQueryClient(s.Conn)
	pr, err := q.Params(s.Ctx, &oraclev2.QueryParamsRequest{})
	require.NoError(s.T, err, "oracle Params")

	params := pr.Params
	if !stringInSlice(params.Sources, s.Env.FunderAddr.String()) {
		params.Sources = append(params.Sources, s.Env.FunderAddr.String())
		msgs = append(msgs, &oraclev2.MsgUpdateParams{Authority: authority, Params: params})
	}

	msgs = append(msgs, &bmev1.MsgFundVault{
		Authority: authority,
		Amount:    sdk.NewCoin(sdkutil.DenomUakt, sdkmath.NewInt(bmeVaultSeedUakt)),
		Source:    s.Env.FunderAddr.String(),
	})
	s.PassGovProposal("grpcsuite: bootstrap uact minting", msgs...)
}

func (s *Suite) waitBMEMintsAllowed() {
	s.T.Helper()

	q := bmev1.NewQueryClient(s.Conn)
	ctx, cancel := context.WithTimeout(s.Ctx, 90*time.Second)
	defer cancel()

	var lastErr error
	var lastStatus *bmev1.QueryStatusResponse
	lastFeedHeight := s.LatestHeight()
	for {
		resp, err := q.Status(ctx, &bmev1.QueryStatusRequest{})
		if err == nil {
			lastStatus = resp
			if resp.MintsAllowed {
				return
			}
		} else {
			lastErr = err
		}

		latest := s.LatestHeight()
		if latest-lastFeedHeight >= 2 {
			s.feedAKTPriceAs(s.Env.Funder, s.Env.FunderAddr, 3)
			lastFeedHeight = s.LatestHeight()
		}

		select {
		case <-ctx.Done():
			if lastStatus != nil {
				s.T.Fatalf("grpcsuite: bme did not allow mints within timeout: status=%s cr=%s last err=%v",
					lastStatus.Status, lastStatus.CollateralRatio, lastErr)
			}
			s.T.Fatalf("grpcsuite: bme did not allow mints within timeout: %v", lastErr)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (s *Suite) waitForFunderUact(target sdk.Coin, timeout time.Duration) bool {
	s.T.Helper()

	ctx, cancel := context.WithTimeout(s.Ctx, timeout)
	defer cancel()

	lastFeedHeight := s.LatestHeight()
	for {
		current := s.balanceOf(s.Env.FunderAddr, sdkutil.DenomUact)
		if current.Amount.GTE(target.Amount) {
			s.logf("funder uact bootstrap complete: %s", current)
			return true
		}

		latest := s.LatestHeight()
		if latest-lastFeedHeight >= 2 {
			s.feedAKTPriceAs(s.Env.Funder, s.Env.FunderAddr, 3)
			lastFeedHeight = s.LatestHeight()
		}

		select {
		case <-ctx.Done():
			return false
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (s *Suite) balanceOf(addr sdk.AccAddress, denom string) sdk.Coin {
	s.T.Helper()

	resp, err := banktypes.NewQueryClient(s.Conn).Balance(s.Ctx, &banktypes.QueryBalanceRequest{
		Address: addr.String(),
		Denom:   denom,
	})
	require.NoErrorf(s.T, err, "bank Balance %s %s", addr, denom)
	if resp.Balance == nil {
		return sdk.NewCoin(denom, sdkmath.ZeroInt())
	}
	return *resp.Balance
}

func stringInSlice(vals []string, target string) bool {
	for _, v := range vals {
		if v == target {
			return true
		}
	}
	return false
}
