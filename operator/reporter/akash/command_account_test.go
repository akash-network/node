package akash

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdksecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
)

func TestCommandAccountPublicKeyBoundary(t *testing.T) {
	for _, name := range []string{"base", "vesting", "no-account", "empty-any", "unknown-any", "malformed-any", "no-pubkey", "ed25519", "invalid-point", "uncompressed", "wrong-key", "wrong-address", "not-found", "height"} {
		t.Run(name, func(t *testing.T) {
			key := sdksecp.GenPrivKeyFromSecret([]byte("R196 auth fixture"))
			addr := sdk.AccAddress(key.PubKey().Address())
			var pub cryptotypes.PubKey = key.PubKey()
			switch name {
			case "no-pubkey":
				pub = nil
			case "ed25519":
				pub = ed25519.GenPrivKey().PubKey()
			case "invalid-point":
				pub = &sdksecp.PubKey{Key: append([]byte{2}, bytes.Repeat([]byte{255}, 32)...)}
			case "uncompressed":
				pub = &sdksecp.PubKey{Key: make([]byte, 65)}
			case "wrong-key":
				pub = sdksecp.GenPrivKey().PubKey()
			}
			base := authtypes.NewBaseAccount(addr, pub, 196, 9)
			if name == "wrong-address" {
				base.Address = sdk.AccAddress(make([]byte, 20)).String()
			}
			var account proto.Message = base
			if name == "vesting" {
				account = &vestingtypes.BaseVestingAccount{BaseAccount: base}
			}
			anyAccount, err := codectypes.NewAnyWithValue(account)
			require.NoError(t, err)
			response := &authtypes.QueryAccountResponse{Account: anyAccount}
			switch name {
			case "no-account":
				response.Account = nil
			case "empty-any":
				response.Account = &codectypes.Any{}
			case "unknown-any":
				response.Account = &codectypes.Any{TypeUrl: "/unknown.Account"}
			case "malformed-any":
				response.Account = &codectypes.Any{
					TypeUrl: "/cosmos.auth.v1beta1.BaseAccount",
					Value:   []byte{255},
				}
			}
			wire, err := response.Marshal()
			require.NoError(t, err)
			calls := 0
			c := mockABCI(t, func(_ *http.Request, query abciWireRequest) abci.ResponseQuery {
				calls++
				require.Equal(t, "/cosmos.auth.v1beta1.Query/Account", query.Path)
				require.Equal(t, int64(42), query.Height)
				var req authtypes.QueryAccountRequest
				require.NoError(t, req.Unmarshal(query.Data))
				require.Equal(t, addr.String(), req.Address)
				out := abci.ResponseQuery{
					Value:  wire,
					Height: 42,
				}
				if name == "not-found" {
					out.Value = nil
					out.Codespace = sdkerrors.RootCodespace
					out.Code = sdkerrors.ErrKeyNotFound.ABCICode()
				} else if name == "height" {
					out.Height = 43
				}
				return out
			})
			// Focus this test on the generated account boundary; fresh-state checks have a separate simnode test.
			c.clock = clock.NewFake(start)
			c.minRefresh = time.Second
			c.track.last = &refreshResult{
				at: start,
				state: &State{
					Height: 42,
				},
			}
			got, err := c.AccountPublicKey(t.Context(), addr)
			require.Equal(t, 1, calls)
			if name == "base" || name == "vesting" {
				require.NoError(t, err)
				require.Equal(t, key.PubKey().Bytes(), got.Bytes())
			} else {
				require.Error(t, err)
				require.Nil(t, got)
				if name == "not-found" {
					require.ErrorIs(t, err, ErrAccountNotFound)
				} else {
					require.NotErrorIs(t, err, ErrAccountNotFound)
				}
				if name == "no-pubkey" {
					require.ErrorContains(t, err, "set externalSigner.publicKeyBase64")
				}
			}
		})
	}
}

func TestCommandAccountFreshSnapshot(t *testing.T) {
	for _, transport := range []string{"abci", "grpc"} {
		for _, stale := range []bool{false, true} {
			t.Run(transport+"/"+map[bool]string{false: "fresh", true: "stale"}[stale], func(t *testing.T) {
				key := sdksecp.GenPrivKeyFromSecret([]byte("R196 auth freshness"))
				addr := sdk.AccAddress(key.PubKey().Address())
				sc := simnode.Scenarios()["clean"]()
				sc.Akash.Accounts = []simnode.AuthAccount{
					{
						Address: addr.String(),
						Number:  196,
						PubKey:  key.PubKey().Bytes(),
					},
				}
				n, fake := startNode(t, sc, simnode.Options{})
				opts := options(n, fake)
				if transport == "abci" {
					opts.ABCI = true
					opts.GRPCAddr = ""
				}
				c := dial(t, opts)
				if stale {
					fake.Advance(2 * time.Minute)
				}
				got, err := c.AccountPublicKey(t.Context(), addr)
				if stale {
					require.Error(t, err)
					require.Nil(t, got)
					require.Zero(t, n.Calls("/cosmos.auth.v1beta1.Query/Account"))
				} else {
					require.NoError(t, err)
					require.Equal(t, base64.StdEncoding.EncodeToString(key.PubKey().Bytes()), base64.StdEncoding.EncodeToString(got.Bytes()))
					if transport == "grpc" {
						require.Positive(t, n.Calls("/cosmos.auth.v1beta1.Query/Account"))
					} else {
						require.Positive(t, n.Calls("abci_query"))
					}
					number, err := c.AccountNumber(t.Context(), addr)
					require.NoError(t, err)
					require.Equal(t, uint64(196), number)
				}
			})
		}
	}
}
