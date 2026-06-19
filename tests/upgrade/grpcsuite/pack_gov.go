package grpcsuite

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	bmev1 "pkg.akt.dev/go/node/bme/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	oraclev2 "pkg.akt.dev/go/node/oracle/v2"
	wasmv1 "pkg.akt.dev/go/node/wasm/v1"
)

// govParamsPack exercises every module's MsgUpdateParams by re-applying each
// module's CURRENT params through a single governance proposal. Re-applying the
// current (already valid) params is a safe no-op on state while still routing the
// message through the real handler — so the gov-gated UpdateParams surface is
// verified without risk of breaking the chain. It runs late so other packs see
// unchanged params.
type govParamsPack struct{}

func (govParamsPack) Name() string { return "gov-params" }

// Available: gov is always present, but gate on it for symmetry.
func (govParamsPack) Available(d *Discovery) bool { return d.HasModule("cosmos.gov.v1") }

func (govParamsPack) Run(s *Suite) {
	authority := s.GovAuthority()
	var msgs []sdk.Msg

	if d := s.Disc; d.HasModule("akash.deployment.v1beta4") {
		p, err := dvbeta.NewQueryClient(s.Conn).Params(s.Ctx, &dvbeta.QueryParamsRequest{})
		if err == nil {
			msgs = append(msgs, &dvbeta.MsgUpdateParams{Authority: authority, Params: p.Params})
		}
	}
	if d := s.Disc; d.HasModule("akash.market.v1beta5") {
		p, err := mvbeta.NewQueryClient(s.Conn).Params(s.Ctx, &mvbeta.QueryParamsRequest{})
		if err == nil {
			msgs = append(msgs, &mvbeta.MsgUpdateParams{Authority: authority, Params: p.Params})
		}
	}
	if d := s.Disc; d.HasModule("akash.oracle.v2") {
		p, err := oraclev2.NewQueryClient(s.Conn).Params(s.Ctx, &oraclev2.QueryParamsRequest{})
		if err == nil {
			msgs = append(msgs, &oraclev2.MsgUpdateParams{Authority: authority, Params: p.Params})
		}
	}
	if d := s.Disc; d.HasModule("akash.bme.v1") {
		p, err := bmev1.NewQueryClient(s.Conn).Params(s.Ctx, &bmev1.QueryParamsRequest{})
		if err == nil {
			msgs = append(msgs, &bmev1.MsgUpdateParams{Authority: authority, Params: p.Params})
		}
	}
	if d := s.Disc; d.HasModule("akash.wasm.v1") {
		p, err := wasmv1.NewQueryClient(s.Conn).Params(s.Ctx, &wasmv1.QueryParamsRequest{})
		if err == nil {
			msgs = append(msgs, &wasmv1.MsgUpdateParams{Authority: authority, Params: p.Params})
		}
	}
	if d := s.Disc; d.HasModule("cosmos.auth.v1beta1") {
		p, err := authtypes.NewQueryClient(s.Conn).Params(s.Ctx, &authtypes.QueryParamsRequest{})
		if err == nil {
			msgs = append(msgs, &authtypes.MsgUpdateParams{Authority: authority, Params: p.Params})
		}
	}
	if d := s.Disc; d.HasModule("cosmos.bank.v1beta1") {
		q := banktypes.NewQueryClient(s.Conn)
		p, err := q.Params(s.Ctx, &banktypes.QueryParamsRequest{})
		if err == nil {
			msgs = append(msgs, &banktypes.MsgUpdateParams{Authority: authority, Params: p.Params})
		}
		msgs = append(msgs, banktypes.NewMsgSetSendEnabled(
			authority,
			[]*banktypes.SendEnabled{{Denom: s.Env.BondDenom, Enabled: true}},
			nil,
		))
	}
	if d := s.Disc; d.HasModule("cosmos.consensus.v1") {
		p, err := consensustypes.NewQueryClient(s.Conn).Params(s.Ctx, &consensustypes.QueryParamsRequest{})
		if err == nil && p.Params != nil {
			msgs = append(msgs, &consensustypes.MsgUpdateParams{
				Authority: authority,
				Block:     p.Params.Block,
				Evidence:  p.Params.Evidence,
				Validator: p.Params.Validator,
				Abci:      p.Params.Abci,
			})
		}
	}
	if d := s.Disc; d.HasModule("cosmos.distribution.v1beta1") {
		p, err := distrtypes.NewQueryClient(s.Conn).Params(s.Ctx, &distrtypes.QueryParamsRequest{})
		if err == nil {
			msgs = append(msgs, &distrtypes.MsgUpdateParams{Authority: authority, Params: p.Params})
		}
	}
	if d := s.Disc; d.HasModule("cosmos.gov.v1") {
		p, err := govv1.NewQueryClient(s.Conn).Params(s.Ctx, &govv1.QueryParamsRequest{})
		if err == nil && p.Params != nil {
			msgs = append(msgs, &govv1.MsgUpdateParams{Authority: authority, Params: govParamsForUpdate(*p.Params)})
		}
	}
	if d := s.Disc; d.HasModule("cosmos.mint.v1beta1") {
		p, err := minttypes.NewQueryClient(s.Conn).Params(s.Ctx, &minttypes.QueryParamsRequest{})
		if err == nil {
			msgs = append(msgs, &minttypes.MsgUpdateParams{Authority: authority, Params: p.Params})
		}
	}
	if d := s.Disc; d.HasModule("cosmos.slashing.v1beta1") {
		p, err := slashingtypes.NewQueryClient(s.Conn).Params(s.Ctx, &slashingtypes.QueryParamsRequest{})
		if err == nil {
			msgs = append(msgs, &slashingtypes.MsgUpdateParams{Authority: authority, Params: p.Params})
		}
	}
	if d := s.Disc; d.HasModule("cosmos.staking.v1beta1") {
		p, err := stakingtypes.NewQueryClient(s.Conn).Params(s.Ctx, &stakingtypes.QueryParamsRequest{})
		if err == nil {
			msgs = append(msgs, &stakingtypes.MsgUpdateParams{Authority: authority, Params: p.Params})
		}
	}

	if len(msgs) == 0 {
		s.logf("gov-params: no MsgUpdateParams modules available")
		return
	}
	s.PassGovProposal("grpcsuite: re-apply module params (no-op)", msgs...)
}
