package client

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/spf13/pflag"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	cmtrpctypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"

	aclient "github.com/akash-network/akash-api/go/node/client"
	"github.com/akash-network/akash-api/go/node/client/v1beta2"
)

var (
	ErrInvalidClient = errors.New("invalid client")
)

func DiscoverQueryClient(ctx context.Context, cctx sdkclient.Context) (v1beta2.QueryClient, error) {
	var cl v1beta2.QueryClient
	err := aclient.DiscoverQueryClient(ctx, cctx, func(i interface{}) error {
		var valid bool

		if cl, valid = i.(v1beta2.QueryClient); !valid {
			return fmt.Errorf("%w: expected %s, actual %s", ErrInvalidClient, reflect.TypeOf(cl), reflect.TypeOf(i))
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return cl, nil
}

func DiscoverClient(ctx context.Context, cctx sdkclient.Context, flags *pflag.FlagSet) (v1beta2.Client, error) {
	var cl v1beta2.Client
	err := aclient.DiscoverClient(ctx, cctx, flags, func(i interface{}) error {
		var valid bool

		if cl, valid = i.(v1beta2.Client); !valid {
			return fmt.Errorf("%w: expected %s, actual %s", ErrInvalidClient, reflect.TypeOf(cl), reflect.TypeOf(i))
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return cl, nil
}

func RPCAkash(_ *cmtrpctypes.Context) (*aclient.Akash, error) {
	result := &aclient.Akash{
		ClientInfo: &aclient.ClientInfo{
			ApiVersion: "v1beta2",
		},
	}

	return result, nil
}
