package grpcsuite

import (
	"fmt"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/stretchr/testify/require"
)

type cosmosGovPack struct{}

func (cosmosGovPack) Name() string { return "cosmos-gov" }

func (cosmosGovPack) Available(d *Discovery) bool { return d.HasModule("cosmos.gov.v1") }

func (cosmosGovPack) Run(s *Suite) {
	gq := govv1.NewQueryClient(s.Conn)
	params, err := gq.Params(s.Ctx, &govv1.QueryParamsRequest{})
	require.NoError(s.T, err, "gov Params")
	require.NotNil(s.T, params.Params, "gov params")
	minDeposit := s.govMinDeposit(gq)
	noop := &govv1.MsgUpdateParams{Authority: s.GovAuthority(), Params: govParamsForUpdate(*params.Params)}

	title := fmt.Sprintf("grpcsuite gov weighted vote %d", s.LatestHeight())
	pid := s.submitGovProposal(gq, title, initialGovDeposit(minDeposit), noop)
	s.BroadcastOK(s.Env.Funder, govv1.NewMsgDeposit(s.Env.FunderAddr, pid, minDeposit))
	s.BroadcastOK(s.Env.Funder, govv1.NewMsgVoteWeighted(
		s.Env.FunderAddr,
		pid,
		govv1.WeightedVoteOptions{govv1.NewWeightedVoteOption(govv1.OptionYes, sdkmath.LegacyOneDec())},
		"grpcsuite weighted yes",
	))

	prop, err := gq.Proposal(s.Ctx, &govv1.QueryProposalRequest{ProposalId: pid})
	require.NoError(s.T, err, "gov Proposal")
	require.Equal(s.T, pid, prop.Proposal.Id)
	props, err := gq.Proposals(s.Ctx, &govv1.QueryProposalsRequest{Pagination: &sdkquery.PageRequest{Limit: 10}})
	require.NoError(s.T, err, "gov Proposals")
	require.NotEmpty(s.T, props.Proposals, "gov proposals should not be empty")
	vote, err := gq.Vote(s.Ctx, &govv1.QueryVoteRequest{ProposalId: pid, Voter: s.Env.FunderAddr.String()})
	require.NoError(s.T, err, "gov Vote")
	require.Equal(s.T, s.Env.FunderAddr.String(), vote.Vote.Voter)
	votes, err := gq.Votes(s.Ctx, &govv1.QueryVotesRequest{
		ProposalId: pid,
		Pagination: &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "gov Votes")
	require.NotEmpty(s.T, votes.Votes, "proposal should have votes")
	dep, err := gq.Deposit(s.Ctx, &govv1.QueryDepositRequest{ProposalId: pid, Depositor: s.Env.FunderAddr.String()})
	require.NoError(s.T, err, "gov Deposit")
	require.NotEmpty(s.T, dep.Deposit.Amount, "proposal deposit should not be empty")
	deps, err := gq.Deposits(s.Ctx, &govv1.QueryDepositsRequest{
		ProposalId: pid,
		Pagination: &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "gov Deposits")
	require.NotEmpty(s.T, deps.Deposits, "proposal should have deposits")
	_, err = gq.TallyResult(s.Ctx, &govv1.QueryTallyResultRequest{ProposalId: pid})
	require.NoError(s.T, err, "gov TallyResult")
	_, err = gq.Constitution(s.Ctx, &govv1.QueryConstitutionRequest{})
	require.NoError(s.T, err, "gov Constitution")
	s.waitProposalPassed(gq, pid)

	cancelTitle := fmt.Sprintf("grpcsuite gov cancel %d", s.LatestHeight())
	cancelID := s.submitGovProposal(gq, cancelTitle, initialGovDeposit(minDeposit), noop)
	s.BroadcastOK(s.Env.Funder, govv1.NewMsgCancelProposal(cancelID, s.Env.FunderAddr.String()))
	_, err = gq.Proposal(s.Ctx, &govv1.QueryProposalRequest{ProposalId: cancelID})
	require.Error(s.T, err, "cancelled proposal should be removed from gov state")

	s.runLegacyGovV1Beta1(gq, minDeposit)

	legacy, err := types.NewAnyWithValue(&govv1beta1.TextProposal{Title: "legacy", Description: "legacy"})
	require.NoError(s.T, err, "pack legacy gov content")
	s.BroadcastExpectErr(s.Env.Funder, govv1.NewMsgExecLegacyContent(legacy, s.GovAuthority()))
	s.BroadcastExpectErr(s.Env.Funder, govv1.NewMsgVote(s.Env.FunderAddr, 9_999_999_999, govv1.OptionYes, "missing proposal"))
	s.logf("cosmos gov complete (submit, deposit, weighted vote, cancel, legacy rejection)")
}

func (s *Suite) submitGovProposal(gq govv1.QueryClient, title string, deposit sdk.Coins, msgs ...sdk.Msg) uint64 {
	s.T.Helper()
	prop, err := govv1.NewMsgSubmitProposal(msgs, deposit, s.Env.FunderAddr.String(), "", title, title, false)
	require.NoError(s.T, err, "build gov proposal")
	lastProposalID := s.latestProposalID(gq)
	res := s.BroadcastOK(s.Env.Funder, prop)
	pid, ok := proposalIDFromEvents(res)
	if !ok {
		pid = s.waitSubmittedProposalID(gq, title, lastProposalID)
	}
	return pid
}

func (s *Suite) runLegacyGovV1Beta1(gq govv1.QueryClient, minDeposit sdk.Coins) {
	s.T.Helper()
	title := fmt.Sprintf("grpcsuite legacy gov %d", s.LatestHeight())
	content := govv1beta1.NewTextProposal(title, title)
	prop, err := govv1beta1.NewMsgSubmitProposal(content, initialGovDeposit(minDeposit), s.Env.FunderAddr)
	require.NoError(s.T, err, "build legacy gov proposal")

	lastProposalID := s.latestProposalID(gq)
	res := s.BroadcastOK(s.Env.Funder, prop)
	pid, ok := proposalIDFromEvents(res)
	if !ok {
		pid = s.waitSubmittedProposalID(gq, title, lastProposalID)
	}
	s.BroadcastOK(s.Env.Funder, govv1beta1.NewMsgDeposit(s.Env.FunderAddr, pid, minDeposit))
	s.BroadcastOK(s.Env.Funder, govv1beta1.NewMsgVote(s.Env.FunderAddr, pid, govv1beta1.OptionYes))
	s.BroadcastOK(s.Env.Funder, govv1beta1.NewMsgVoteWeighted(
		s.Env.FunderAddr,
		pid,
		govv1beta1.NewNonSplitVoteOption(govv1beta1.OptionYes),
	))
	s.waitProposalPassed(gq, pid)
}

func initialGovDeposit(min sdk.Coins) sdk.Coins {
	if len(min) == 0 {
		return nil
	}
	coin := min[0]
	if coin.Amount.IsZero() {
		return nil
	}
	if coin.Amount.GT(sdkmath.OneInt()) {
		coin.Amount = coin.Amount.QuoRaw(2)
		if coin.Amount.IsZero() {
			coin.Amount = sdkmath.OneInt()
		}
	}
	return sdk.NewCoins(coin)
}

func govParamsForUpdate(params govv1.Params) govv1.Params {
	if params.VotingPeriod == nil || params.VotingPeriod.Seconds() <= 0 {
		votingPeriod := 2 * time.Minute
		params.VotingPeriod = &votingPeriod
	}

	if params.ExpeditedVotingPeriod == nil || params.ExpeditedVotingPeriod.Seconds() <= 0 ||
		params.ExpeditedVotingPeriod.Seconds() >= params.VotingPeriod.Seconds() {
		expeditedVotingPeriod := *params.VotingPeriod / 2
		if expeditedVotingPeriod < time.Second {
			votingPeriod := 2 * time.Second
			expeditedVotingPeriod = time.Second
			params.VotingPeriod = &votingPeriod
		}
		params.ExpeditedVotingPeriod = &expeditedVotingPeriod
	}

	return params
}
