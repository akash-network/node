package keeper_test

import (
	"context"
	"encoding/base64"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
)

func TestGRPCQueryOrdersRejectsLegacyPaginationKey(t *testing.T) {
	suite := setupTest(t)

	// A pre-collections cursor has a valid envelope but encodes a length-prefixed
	// raw owner address instead of the current string key. This cursor reproduces
	// the provider's "expected buffer of size 4" error.
	key, err := base64.StdEncoding.DecodeString("jVFQowEBAxEAASUUEREREREREREREREREREREREREREAAAAAAoDegAAAAAEAAAAB")
	require.NoError(t, err)
	req := &mvbeta.QueryOrdersRequest{
		Filters: mvbeta.OrderFilters{
			State: mvbeta.OrderOpen.String(),
		},
		Pagination: &sdkquery.PageRequest{
			Key:   key,
			Limit: 1,
		},
	}

	t.Run("gRPC", func(t *testing.T) {
		_, err := suite.queryClient.Orders(suite.ctx, req)
		require.ErrorContains(t, err, "collections: encoding error: expected buffer of size 4")
		require.Equal(t, codes.InvalidArgument, status.Code(err), err)
	})

	t.Run("ABCI", func(t *testing.T) {
		_, err := suite.App().FinalizeBlock(&abci.RequestFinalizeBlock{
			Height: 1,
		})
		require.NoError(t, err)
		_, err = suite.App().Commit()
		require.NoError(t, err)

		data, err := req.Marshal()
		require.NoError(t, err)
		res, err := suite.App().Query(context.Background(), &abci.RequestQuery{
			Path: "/akash.market.v1beta5.Query/Orders",
			Data: data,
		})
		require.NoError(t, err)
		require.Contains(t, res.Log, "collections: encoding error: expected buffer of size 4")
		require.Equal(t, sdkerrors.ErrInvalidRequest.ABCICode(), res.Code, res.Log)
	})
}
