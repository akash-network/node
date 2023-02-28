package keeper_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"

	"github.com/akash-network/node/app"
	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/x/cert/keeper"
)

type grpcTestSuite struct {
	t       *testing.T
	app     *app.AkashApp
	ctx     sdk.Context
	keeper  keeper.Keeper
	qclient types.QueryClient
}

func setupTest(t *testing.T) *grpcTestSuite {
	suite := &grpcTestSuite{
		t: t,
	}

	suite.app = app.Setup(false)
	suite.ctx, suite.keeper = setupKeeper(t)
	querier := suite.keeper.Querier()

	queryHelper := baseapp.NewQueryServerTestHelper(suite.ctx, suite.app.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, querier)
	suite.qclient = types.NewQueryClient(queryHelper)

	return suite
}

func sortCerts(certs types.Certificates) {
	sort.SliceStable(certs, func(i, j int) bool {
		return certs[i].State < certs[j].State
	})
}

func TestCertGRPCQueryCertificates(t *testing.T) {
	suite := setupTest(t)

	owner := testutil.AccAddress(t)
	cert := testutil.Certificate(t, owner)

	owner2 := testutil.AccAddress(t)
	cert2 := testutil.Certificate(t, owner2)

	err := suite.keeper.CreateCertificate(suite.ctx, owner, cert.PEM.Cert, cert.PEM.Pub)
	require.NoError(t, err)

	err = suite.keeper.CreateCertificate(suite.ctx, owner2, cert2.PEM.Cert, cert2.PEM.Pub)
	require.NoError(t, err)

	err = suite.keeper.RevokeCertificate(suite.ctx, types.CertID{
		Owner:  owner2,
		Serial: cert2.Serial,
	})
	require.NoError(t, err)

	var req *types.QueryCertificatesRequest
	var expCertificates types.Certificates

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"all certificates",
			func() {
				req = &types.QueryCertificatesRequest{}
				expCertificates = types.Certificates{
					types.Certificate{
						State:  types.CertificateValid,
						Cert:   cert.PEM.Cert,
						Pubkey: cert.PEM.Pub,
					},
					types.Certificate{
						State:  types.CertificateRevoked,
						Cert:   cert2.PEM.Cert,
						Pubkey: cert2.PEM.Pub,
					},
				}
			},
			true,
		},
		{
			"certificate not found",
			func() {
				req = &types.QueryCertificatesRequest{
					Filter: types.CertificateFilter{
						Owner: testutil.AccAddress(t).String(),
					},
				}

				expCertificates = nil
			},
			false,
		},
		{
			"success valid",
			func() {
				req = &types.QueryCertificatesRequest{
					Filter: types.CertificateFilter{
						Owner: owner.String(),
					},
				}
				expCertificates = types.Certificates{
					types.Certificate{
						State:  types.CertificateValid,
						Cert:   cert.PEM.Cert,
						Pubkey: cert.PEM.Pub,
					},
				}
			},
			true,
		},
		{
			"success revoked",
			func() {
				req = &types.QueryCertificatesRequest{
					Filter: types.CertificateFilter{
						Owner: owner2.String(),
					},
				}
				expCertificates = types.Certificates{
					types.Certificate{
						State:  types.CertificateRevoked,
						Cert:   cert2.PEM.Cert,
						Pubkey: cert2.PEM.Pub,
					},
				}
			},
			true,
		},
		{
			"success pagination with limit",
			func() {
				req = &types.QueryCertificatesRequest{
					Pagination: &sdkquery.PageRequest{
						Limit: 10,
					},
				}
				expCertificates = types.Certificates{
					types.Certificate{
						State:  types.CertificateValid,
						Cert:   cert.PEM.Cert,
						Pubkey: cert.PEM.Pub,
					},
					types.Certificate{
						State:  types.CertificateRevoked,
						Cert:   cert2.PEM.Cert,
						Pubkey: cert2.PEM.Pub,
					},
				}
			},
			true,
		},
		{
			"success pagination with limit with state",
			func() {
				req = &types.QueryCertificatesRequest{
					Filter: types.CertificateFilter{
						State: types.Certificate_State_name[int32(types.CertificateValid)],
					},
					Pagination: &sdkquery.PageRequest{
						Limit: 10,
					},
				}
				expCertificates = types.Certificates{
					types.Certificate{
						State:  types.CertificateValid,
						Cert:   cert.PEM.Cert,
						Pubkey: cert.PEM.Pub,
					},
				}
			},
			true,
		},
		{
			"success pagination with limit with owner",
			func() {
				req = &types.QueryCertificatesRequest{
					Filter: types.CertificateFilter{
						Owner: owner2.String(),
					},
					Pagination: &sdkquery.PageRequest{
						Limit: 10,
					},
				}
				expCertificates = types.Certificates{
					types.Certificate{
						State:  types.CertificateRevoked,
						Cert:   cert2.PEM.Cert,
						Pubkey: cert2.PEM.Pub,
					},
				}
			},
			true,
		},
		{
			"failing pagination with limit with non-existing owner",
			func() {
				req = &types.QueryCertificatesRequest{
					Filter: types.CertificateFilter{
						Owner: testutil.AccAddress(t).String(),
					},
					Pagination: &sdkquery.PageRequest{
						Limit: 10,
					},
				}
				expCertificates = nil
			},
			false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := sdk.WrapSDKContext(suite.ctx)

			res, err := suite.qclient.Certificates(ctx, req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				if expCertificates != nil {
					sortCerts(expCertificates)

					respCerts := make(types.Certificates, len(res.Certificates))
					for i, cert := range res.Certificates {
						respCerts[i] = cert.Certificate
					}

					sortCerts(respCerts)
					require.Equal(t, expCertificates, respCerts)
				}
			} else {
				require.NotNil(t, res)
			}
		})
	}
}
