package client

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	cmtrpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"

	aclient "pkg.akt.dev/go/node/client"
	cltypes "pkg.akt.dev/go/node/client/types"
	"pkg.akt.dev/go/node/client/v1beta3"
)

var (
	ErrInvalidClient = errors.New("invalid client")
)

func DiscoverQueryClient(ctx context.Context, cctx sdkclient.Context) (v1beta3.QueryClient, error) {
	var cl v1beta3.QueryClient
	err := aclient.DiscoverQueryClient(ctx, cctx, func(i interface{}) error {
		var valid bool

		if cl, valid = i.(v1beta3.QueryClient); !valid {
			return fmt.Errorf("%w: expected %s, actual %s", ErrInvalidClient, reflect.TypeOf(cl), reflect.TypeOf(i))
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return cl, nil
}

func DiscoverClient(ctx context.Context, cctx sdkclient.Context, opts ...cltypes.ClientOption) (v1beta3.Client, error) {
	var cl v1beta3.Client

	setupFn := func(i interface{}) error {
		var valid bool

		if cl, valid = i.(v1beta3.Client); !valid {
			return fmt.Errorf("%w: expected %s, actual %s", ErrInvalidClient, reflect.TypeOf(cl), reflect.TypeOf(i))
		}

		return nil
	}

	err := aclient.DiscoverClient(ctx, cctx, setupFn, opts...)

	if err != nil {
		return nil, err
	}

	return cl, nil
}

func RPCAkash(_ *cmtrpctypes.Context) (*aclient.Akash, error) {
	result := &aclient.Akash{
		ClientInfo: &aclient.ClientInfo{
			ApiVersion: "v1beta3",
		},
	}

	return result, nil
}
