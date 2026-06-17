package grpcsuite

import (
	"context"
	"strconv"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/stretchr/testify/require"
)

// GovAuthority returns the bech32 address of the gov module account, which is the
// `authority` for every MsgUpdateParams and other gov-gated message.
func (s *Suite) GovAuthority() string {
	s.T.Helper()
	if v := s.World.Get("gov.authority"); v != nil {
		return v.(string)
	}
	qc := authtypes.NewQueryClient(s.Conn)
	resp, err := qc.ModuleAccountByName(s.Ctx, &authtypes.QueryModuleAccountByNameRequest{Name: "gov"})
	require.NoError(s.T, err, "query gov module account")

	var acc sdk.AccountI
	require.NoError(s.T, s.Env.InterfaceReg.UnpackAny(resp.Account, &acc))
	addr := acc.GetAddress().String()
	s.World.Set("gov.authority", addr)
	return addr
}

// PassGovProposal submits msgs as a single governance proposal from the Funder,
// votes yes, waits for it to pass and execute, then records each wrapped message for
// coverage. The Funder must hold ~all voting power — true on a testnetify fork and
// on the single-validator in-process network. Requires a short voting period
// (configured in the testnetify state / in-process genesis).
func (s *Suite) PassGovProposal(title string, msgs ...sdk.Msg) {
	s.T.Helper()
	require.NotEmpty(s.T, msgs, "PassGovProposal needs at least one message")

	gq := govv1.NewQueryClient(s.Conn)

	deposit := s.govMinDeposit(gq)
	prop, err := govv1.NewMsgSubmitProposal(msgs, deposit, s.Env.FunderAddr.String(), "", title, title, false)
	require.NoError(s.T, err, "build gov proposal")

	res := s.BroadcastOK(s.Env.Funder, prop)
	pid := proposalIDFromEvents(s.T, res)

	s.BroadcastOK(s.Env.Funder, govv1.NewMsgVote(s.Env.FunderAddr, pid, govv1.VoteOption_VOTE_OPTION_YES, ""))
	s.waitProposalPassed(gq, pid)

	// Wrapped messages executed on-chain when the proposal passed; count them.
	for _, m := range msgs {
		s.Cov.recordMsg(sdk.MsgTypeURL(m))
	}
	s.logf("gov proposal %d passed (%d msg(s)): %s", pid, len(msgs), title)
}

func (s *Suite) govMinDeposit(gq govv1.QueryClient) sdk.Coins {
	resp, err := gq.Params(s.Ctx, &govv1.QueryParamsRequest{ParamsType: "deposit"})
	if err == nil && resp.Params != nil && len(resp.Params.MinDeposit) > 0 {
		return sdk.NewCoins(resp.Params.MinDeposit...)
	}
	return sdk.NewCoins(sdk.NewInt64Coin(s.Env.BondDenom, 10_000_000))
}

func (s *Suite) waitProposalPassed(gq govv1.QueryClient, pid uint64) {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, 120*time.Second)
	defer cancel()
	for {
		resp, err := gq.Proposal(ctx, &govv1.QueryProposalRequest{ProposalId: pid})
		if err == nil && resp.Proposal != nil {
			switch resp.Proposal.Status {
			case govv1.ProposalStatus_PROPOSAL_STATUS_PASSED:
				return
			case govv1.ProposalStatus_PROPOSAL_STATUS_REJECTED, govv1.ProposalStatus_PROPOSAL_STATUS_FAILED:
				s.T.Fatalf("gov proposal %d ended in status %s", pid, resp.Proposal.Status)
			}
		}
		select {
		case <-ctx.Done():
			s.T.Fatalf("gov proposal %d did not pass within timeout (last err: %v)", pid, err)
		case <-time.After(time.Second):
		}
	}
}

// proposalIDFromEvents extracts the proposal_id emitted by a MsgSubmitProposal tx.
func proposalIDFromEvents(t *testing.T, res *sdk.TxResponse) uint64 {
	t.Helper()
	for _, ev := range res.Events {
		for _, a := range ev.Attributes {
			if a.Key == "proposal_id" {
				id, err := strconv.ParseUint(a.Value, 10, 64)
				require.NoErrorf(t, err, "parse proposal_id %q", a.Value)
				return id
			}
		}
	}
	t.Fatalf("proposal_id not found in submit-proposal tx events")
	return 0
}
