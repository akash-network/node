package grpcsuite

import (
	"path/filepath"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	depositv1 "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/sdkutil"
	"pkg.akt.dev/go/sdl"
)

// World keys published by the deployment pack for downstream packs (market, escrow).
const (
	wDeploymentSigner = "deployment.signer" // keyring name of the tenant
	wDeploymentID     = "deployment.id"     // dv1.DeploymentID of an open deployment
	wDeploymentGSeq   = "deployment.gseq"   // uint32 GSeq of its first group
)

type deploymentPack struct{}

func (deploymentPack) Name() string { return "deployment" }

func (deploymentPack) Available(d *Discovery) bool {
	return d.HasModule("akash.deployment.v1beta4")
}

func (dp deploymentPack) Run(s *Suite) {
	q := dvbeta.NewQueryClient(s.Conn)

	// Setup: a funded tenant account.
	tenant := s.FundAccountDefault("tenant")

	// Query Params (also sizes the deposit).
	pr, err := q.Params(s.Ctx, &dvbeta.QueryParamsRequest{})
	require.NoError(s.T, err, "deployment Params")
	deposit := deploymentMinDeposit(s, pr.Params)

	// MsgCreateDeployment from the repo's deployment SDL testdata.
	groups, version := readSDLGroups(s, deploymentSDLPath(s))
	depID := dv1.DeploymentID{Owner: tenant.String(), DSeq: s.NextDSeq()}
	create := &dvbeta.MsgCreateDeployment{
		ID:      depID,
		Groups:  groups,
		Hash:    version,
		Deposit: depositv1.Deposit{Amount: deposit, Sources: depositv1.Sources{depositv1.SourceBalance}},
	}
	s.BroadcastOK("tenant", create)
	s.logf("created deployment %s/%d (%d group(s))", depID.Owner, depID.DSeq, len(groups))

	// Query Deployment by ID.
	depResp, err := q.Deployment(s.Ctx, &dvbeta.QueryDeploymentRequest{ID: depID})
	require.NoError(s.T, err, "Deployment by id")
	require.NotEmpty(s.T, depResp.Groups, "created deployment should have groups")
	gseq := depResp.Groups[0].ID.GSeq

	// Query Group by ID.
	_, err = q.Group(s.Ctx, &dvbeta.QueryGroupRequest{
		ID: dv1.GroupID{Owner: depID.Owner, DSeq: depID.DSeq, GSeq: gseq},
	})
	require.NoError(s.T, err, "Group by id")

	// Query Deployments filtered by owner.
	list, err := q.Deployments(s.Ctx, &dvbeta.QueryDeploymentsRequest{
		Filters:    dvbeta.DeploymentFilters{Owner: depID.Owner},
		Pagination: &sdkquery.PageRequest{Limit: 100},
	})
	require.NoError(s.T, err, "Deployments")
	require.NotEmpty(s.T, list.Deployments, "owner should have at least one deployment")

	// Publish handles so the market/escrow packs can use this OPEN deployment.
	s.World.Set(wDeploymentSigner, "tenant")
	s.World.Set(wDeploymentID, depID)
	s.World.Set(wDeploymentGSeq, gseq)

	// Exercise the rest of the deployment lifecycle on throwaway deployments so the
	// primary one above stays open for the market pack.
	dp.runLifecycle(s, groups, version, deposit)
}

// runLifecycle exercises MsgUpdateDeployment, MsgPauseGroup, MsgStartGroup,
// MsgCloseGroup and MsgCloseDeployment on scratch deployments owned by the tenant.
func (dp deploymentPack) runLifecycle(s *Suite, groups dvbeta.GroupSpecs, version []byte, deposit sdk.Coin) {
	mkDeposit := func() depositv1.Deposit {
		return depositv1.Deposit{Amount: deposit, Sources: depositv1.Sources{depositv1.SourceBalance}}
	}
	create := func() dv1.DeploymentID {
		id := dv1.DeploymentID{Owner: s.Addr("tenant").String(), DSeq: s.NextDSeq()}
		s.BroadcastOK("tenant", &dvbeta.MsgCreateDeployment{ID: id, Groups: groups, Hash: version, Deposit: mkDeposit()})
		return id
	}

	// Scratch deployment #1: update + group pause/start/close.
	scratch := create()
	s.BroadcastOK("tenant", &dvbeta.MsgUpdateDeployment{ID: scratch, Hash: bumpHash(version)})
	gid := dv1.GroupID{Owner: scratch.Owner, DSeq: scratch.DSeq, GSeq: 1}
	s.BroadcastOK("tenant", &dvbeta.MsgPauseGroup{ID: gid})
	s.BroadcastOK("tenant", &dvbeta.MsgStartGroup{ID: gid})
	s.BroadcastOK("tenant", &dvbeta.MsgCloseGroup{ID: gid})

	// Scratch deployment #2: close the whole deployment.
	scratch2 := create()
	s.BroadcastOK("tenant", &dvbeta.MsgCloseDeployment{ID: scratch2})
	s.logf("deployment lifecycle complete (update/pause/start/closeGroup/closeDeployment)")
}

// bumpHash returns a distinct 32-byte hash derived from src so MsgUpdateDeployment
// represents an actual change.
func bumpHash(src []byte) []byte {
	out := make([]byte, len(src))
	copy(out, src)
	if len(out) > 0 {
		out[0] ^= 0xFF
	}
	return out
}

func deploymentSDLPath(s *Suite) string {
	return filepath.Join(s.Env.RepoRoot, "x", "deployment", "testdata", "deployment.yaml")
}

// createDeploymentFromSDL creates a fresh deployment owned by the keyring account
// `signer` from the repo's SDL testdata and returns its ID. Used by the market pack
// to obtain additional orders.
func createDeploymentFromSDL(s *Suite, signer string) dv1.DeploymentID {
	s.T.Helper()
	pr, err := dvbeta.NewQueryClient(s.Conn).Params(s.Ctx, &dvbeta.QueryParamsRequest{})
	require.NoError(s.T, err, "deployment Params")
	groups, version := readSDLGroups(s, deploymentSDLPath(s))
	id := dv1.DeploymentID{Owner: s.Addr(signer).String(), DSeq: s.NextDSeq()}
	s.BroadcastOK(signer, &dvbeta.MsgCreateDeployment{
		ID:      id,
		Groups:  groups,
		Hash:    version,
		Deposit: depositv1.Deposit{Amount: deploymentMinDeposit(s, pr.Params), Sources: depositv1.Sources{depositv1.SourceBalance}},
	})
	return id
}

func readSDLGroups(s *Suite, path string) (dvbeta.GroupSpecs, []byte) {
	s.T.Helper()
	m, err := sdl.ReadFile(path)
	require.NoErrorf(s.T, err, "sdl.ReadFile %s", path)
	groups, err := m.DeploymentGroups()
	require.NoError(s.T, err, "DeploymentGroups")
	version, err := m.Version()
	require.NoError(s.T, err, "Version")
	return groups, version
}

func deploymentMinDeposit(s *Suite, p dvbeta.Params) sdk.Coin {
	if c, err := p.MinDepositFor(sdkutil.DenomUact); err == nil && !c.IsZero() {
		return c
	}
	return sdk.NewCoin(sdkutil.DenomUact, sdkmath.NewInt(5_000_000))
}
