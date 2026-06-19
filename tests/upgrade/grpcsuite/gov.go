package grpcsuite

import (
	"context"
	"strconv"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
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

	lastProposalID := s.latestProposalID(gq)
	res := s.BroadcastOK(s.Env.Funder, prop)
	pid, ok := proposalIDFromEvents(res)
	if !ok {
		pid = s.waitSubmittedProposalID(gq, title, lastProposalID)
	}

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

func (s *Suite) latestProposalID(gq govv1.QueryClient) uint64 {
	s.T.Helper()

	var maxID uint64
	err := s.eachProposal(s.Ctx, gq, func(p *govv1.Proposal) bool {
		if p.Id > maxID {
			maxID = p.Id
		}
		return false
	})
	require.NoError(s.T, err, "query gov proposals before submit")
	return maxID
}

func (s *Suite) waitSubmittedProposalID(gq govv1.QueryClient, title string, after uint64) uint64 {
	s.T.Helper()

	ctx, cancel := context.WithTimeout(s.Ctx, 20*time.Second)
	defer cancel()

	var lastErr error
	for {
		var found uint64
		err := s.eachProposal(ctx, gq, func(p *govv1.Proposal) bool {
			if p.Id > after && p.Title == title && p.Proposer == s.Env.FunderAddr.String() {
				found = p.Id
				return true
			}
			return false
		})
		if err == nil && found != 0 {
			return found
		}
		if err != nil {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			s.T.Fatalf("gov proposal %q submitted but was not found in gov state after proposal %d: %v", title, after, lastErr)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (s *Suite) eachProposal(ctx context.Context, gq govv1.QueryClient, visit func(*govv1.Proposal) bool) error {
	s.T.Helper()

	var next []byte
	for {
		resp, err := gq.Proposals(ctx, &govv1.QueryProposalsRequest{
			Pagination: &sdkquery.PageRequest{Key: next, Limit: 100},
		})
		if err != nil {
			return err
		}
		for _, p := range resp.Proposals {
			if p != nil && visit(p) {
				return nil
			}
		}
		if resp.Pagination == nil || len(resp.Pagination.NextKey) == 0 {
			return nil
		}
		next = resp.Pagination.NextKey
	}
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

// proposalIDFromEvents extracts the proposal_id emitted by a MsgSubmitProposal tx
// when tx indexing is enabled. Forked upgrade nodes may disable tx indexing, so
// callers must fall back to gov state when this returns false.
func proposalIDFromEvents(res *sdk.TxResponse) (uint64, bool) {
	if res == nil {
		return 0, false
	}
	for _, ev := range res.Events {
		for _, a := range ev.Attributes {
			if a.Key == "proposal_id" {
				id, err := strconv.ParseUint(a.Value, 10, 64)
				if err == nil {
					return id, true
				}
			}
		}
	}
	return 0, false
}
