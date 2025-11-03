package keeper_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	types "pkg.akt.dev/go/node/cert/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/app"
	"pkg.akt.dev/node/v2/x/cert/keeper"
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

	suite.app = app.Setup(app.WithGenesis(app.GenesisStateWithValSet))

	suite.ctx, suite.keeper = setupKeeper(t)
	querier := suite.keeper.Querier()

	queryHelper := baseapp.NewQueryServerTestHelper(suite.ctx, suite.app.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, querier)
	suite.qclient = types.NewQueryClient(queryHelper)

	return suite
}

func sortCerts(certs types.Certificates) {
	sort.SliceStable(certs, func(i, j int) bool {
		if certs[i].State < certs[j].State {
			return true
		}

		return string(certs[i].Cert) < string(certs[j].Cert)
	})
}

func TestCertGRPCQueryCertificates(t *testing.T) {
	suite := setupTest(t)

	owner := testutil.AccAddress(t)
	owner2 := testutil.AccAddress(t)
	owner3 := testutil.AccAddress(t)

	cert := testutil.Certificate(t, owner)
	cert2 := testutil.Certificate(t, owner2)
	cert3 := testutil.Certificate(t, owner3)

	err := suite.keeper.CreateCertificate(suite.ctx, owner, cert.PEM.Cert, cert.PEM.Pub)
	require.NoError(t, err)

	err = suite.keeper.CreateCertificate(suite.ctx, owner2, cert2.PEM.Cert, cert2.PEM.Pub)
	require.NoError(t, err)

	err = suite.keeper.CreateCertificate(suite.ctx, owner3, cert3.PEM.Cert, cert3.PEM.Pub)
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
		nextKey  bool
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
						State:  types.CertificateValid,
						Cert:   cert3.PEM.Cert,
						Pubkey: cert3.PEM.Pub,
					},
					types.Certificate{
						State:  types.CertificateRevoked,
						Cert:   cert2.PEM.Cert,
						Pubkey: cert2.PEM.Pub,
					},
				}
			},
			true,
			false,
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
			false,
		},
		{
			"success revoked by owner",
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
			false,
		},
		{
			"success revoked by state",
			func() {
				req = &types.QueryCertificatesRequest{
					Filter: types.CertificateFilter{
						State: types.CertificateRevoked.String(),
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
			false,
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
						State:  types.CertificateValid,
						Cert:   cert3.PEM.Cert,
						Pubkey: cert3.PEM.Pub,
					},
					types.Certificate{
						State:  types.CertificateRevoked,
						Cert:   cert2.PEM.Cert,
						Pubkey: cert2.PEM.Pub,
					},
				}
			},
			true,
			false,
		},

		// {
		// 	"success pagination with next key",
		// 	func() {
		// 		req = &types.QueryCertificatesRequest{
		// 			Filter: types.CertificateFilter{State: types.CertificateValid.String()},
		// 			Pagination: &sdkquery.PageRequest{
		// 				Limit: 1,
		// 			},
		// 		}
		// 		expCertificates = types.Certificates{
		// 			types.Certificate{
		// 				State:  types.CertificateValid,
		// 				Cert:   cert.PEM.Cert,
		// 				Pubkey: cert.PEM.Pub,
		// 			},
		// 		}
		// 	},
		// 	true,
		// 	true,
		// },

		{
			"success pagination with nil key",
			func() {
				req = &types.QueryCertificatesRequest{
					Filter: types.CertificateFilter{State: types.CertificateRevoked.String()},
					Pagination: &sdkquery.PageRequest{
						Limit: 1,
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
			false,
		},
		{
			"success pagination with limit with state",
			func() {
				req = &types.QueryCertificatesRequest{
					Filter: types.CertificateFilter{
						State: types.CertificateValid.String(),
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
					types.Certificate{
						State:  types.CertificateValid,
						Cert:   cert3.PEM.Cert,
						Pubkey: cert3.PEM.Pub,
					},
				}
			},
			true,
			false,
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
			false,
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
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := suite.ctx

			res, err := suite.qclient.Certificates(ctx, req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				if expCertificates != nil {
					sortCerts(expCertificates)

					respCerts := make(types.Certificates, 0, len(res.Certificates))
					for _, cert := range res.Certificates {
						respCerts = append(respCerts, cert.Certificate)
					}

					sortCerts(respCerts)

					if req.Pagination != nil && req.Pagination.Limit > 0 {
						require.LessOrEqual(t, len(respCerts), int(req.Pagination.Limit)) //nolint:gosec
					}

					require.Len(t, respCerts, len(expCertificates))

					for i, cert := range expCertificates {
						require.Equal(t, cert, respCerts[i])
					}
				}

				if tc.nextKey {
					require.NotNil(t, res.Pagination.NextKey)

					req.Pagination.Key = res.Pagination.NextKey
					res, err = suite.qclient.Certificates(ctx, req)
					require.NoError(t, err)
					require.NotNil(t, res)
					if req.Pagination != nil && req.Pagination.Limit > 0 {
						require.LessOrEqual(t, len(res.Certificates), int(req.Pagination.Limit)) //nolint:gosec
					}

					require.Nil(t, res.Pagination.NextKey)
				}
			} else {
				require.NotNil(t, res)
			}
		})
	}
}
