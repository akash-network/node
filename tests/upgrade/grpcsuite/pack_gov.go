package grpcsuite

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

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

	if len(msgs) == 0 {
		s.logf("gov-params: no MsgUpdateParams modules available")
		return
	}
	s.PassGovProposal("grpcsuite: re-apply module params (no-op)", msgs...)
}
