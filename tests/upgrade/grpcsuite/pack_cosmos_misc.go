package grpcsuite

import (
	evidencetypes "cosmossdk.io/x/evidence/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramproposal "github.com/cosmos/cosmos-sdk/x/params/types/proposal"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/stretchr/testify/require"
)

type cosmosMiscPack struct{}

func (cosmosMiscPack) Name() string { return "cosmos-misc" }

func (cosmosMiscPack) Available(d *Discovery) bool {
	return d.HasModule("cosmos.mint.v1beta1") ||
		d.HasModule("cosmos.consensus.v1") ||
		d.HasModule("cosmos.params.v1beta1") ||
		d.HasModule("cosmos.slashing.v1beta1") ||
		d.HasModule("cosmos.evidence.v1beta1") ||
		d.HasModule("cosmos.upgrade.v1beta1")
}

func (cosmosMiscPack) Run(s *Suite) {
	if s.Disc.HasModule("cosmos.mint.v1beta1") {
		q := minttypes.NewQueryClient(s.Conn)
		_, err := q.Params(s.Ctx, &minttypes.QueryParamsRequest{})
		require.NoError(s.T, err, "mint Params")
		_, err = q.Inflation(s.Ctx, &minttypes.QueryInflationRequest{})
		require.NoError(s.T, err, "mint Inflation")
		_, err = q.AnnualProvisions(s.Ctx, &minttypes.QueryAnnualProvisionsRequest{})
		require.NoError(s.T, err, "mint AnnualProvisions")
	}

	if s.Disc.HasModule("cosmos.consensus.v1") {
		q := consensustypes.NewQueryClient(s.Conn)
		_, err := q.Params(s.Ctx, &consensustypes.QueryParamsRequest{})
		require.NoError(s.T, err, "consensus Params")
	}

	if s.Disc.HasModule("cosmos.params.v1beta1") {
		q := paramproposal.NewQueryClient(s.Conn)
		_, err := q.Subspaces(s.Ctx, &paramproposal.QuerySubspacesRequest{})
		require.NoError(s.T, err, "params Subspaces")
		_, _ = q.Params(s.Ctx, &paramproposal.QueryParamsRequest{Subspace: "staking", Key: "BondDenom"})
	}

	if s.Disc.HasModule("cosmos.slashing.v1beta1") {
		q := slashingtypes.NewQueryClient(s.Conn)
		val := s.primaryValidator()
		_, err := q.Params(s.Ctx, &slashingtypes.QueryParamsRequest{})
		require.NoError(s.T, err, "slashing Params")
		infos, err := q.SigningInfos(s.Ctx, &slashingtypes.QuerySigningInfosRequest{Pagination: &sdkquery.PageRequest{Limit: 10}})
		require.NoError(s.T, err, "slashing SigningInfos")
		if len(infos.Info) > 0 {
			_, err = q.SigningInfo(s.Ctx, &slashingtypes.QuerySigningInfoRequest{ConsAddress: infos.Info[0].Address})
			require.NoError(s.T, err, "slashing SigningInfo")
		} else {
			_, err = q.SigningInfo(s.Ctx, &slashingtypes.QuerySigningInfoRequest{ConsAddress: s.primaryValidatorConsAddress(val)})
			require.NoError(s.T, err, "slashing SigningInfo(primary validator)")
		}
		s.BroadcastTolerant(s.Env.Funder, []string{"not jailed", "cannot be unjailed", "validator still jailed"},
			slashingtypes.NewMsgUnjail(val.OperatorAddress))
	}

	if s.Disc.HasModule("cosmos.evidence.v1beta1") {
		q := evidencetypes.NewQueryClient(s.Conn)
		_, err := q.AllEvidence(s.Ctx, &evidencetypes.QueryAllEvidenceRequest{Pagination: &sdkquery.PageRequest{Limit: 10}})
		require.NoError(s.T, err, "evidence AllEvidence")
		_, err = q.Evidence(s.Ctx, &evidencetypes.QueryEvidenceRequest{Hash: "00"})
		require.Error(s.T, err, "unknown evidence hash should be rejected")
		s.BroadcastExpectErr(s.Env.Funder, &evidencetypes.MsgSubmitEvidence{Submitter: s.Env.FunderAddr.String()})
	}

	if s.Disc.HasModule("cosmos.upgrade.v1beta1") {
		q := upgradetypes.NewQueryClient(s.Conn)
		_, err := q.CurrentPlan(s.Ctx, &upgradetypes.QueryCurrentPlanRequest{})
		require.NoError(s.T, err, "upgrade CurrentPlan")
		versions, err := q.ModuleVersions(s.Ctx, &upgradetypes.QueryModuleVersionsRequest{})
		require.NoError(s.T, err, "upgrade ModuleVersions")
		require.NotEmpty(s.T, versions.ModuleVersions, "module versions should not be empty")
		authority, err := q.Authority(s.Ctx, &upgradetypes.QueryAuthorityRequest{})
		require.NoError(s.T, err, "upgrade Authority")
		require.Equal(s.T, s.GovAuthority(), authority.Address)
		_, _ = q.AppliedPlan(s.Ctx, &upgradetypes.QueryAppliedPlanRequest{Name: "grpcsuite-missing-plan"})
		_, _ = q.UpgradedConsensusState(s.Ctx, &upgradetypes.QueryUpgradedConsensusStateRequest{LastHeight: s.LatestHeight()})
		s.BroadcastExpectErr(s.Env.Funder, &upgradetypes.MsgSoftwareUpgrade{
			Authority: s.GovAuthority(),
			Plan: upgradetypes.Plan{
				Name:   "grpcsuite-direct-upgrade",
				Height: s.LatestHeight() + 100_000,
			},
		})
		s.BroadcastExpectErr(s.Env.Funder, &upgradetypes.MsgCancelUpgrade{Authority: s.GovAuthority()})
	}

	s.logf("cosmos misc complete (mint, consensus, params, slashing, evidence, upgrade)")
}
