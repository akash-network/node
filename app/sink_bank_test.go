package app_test

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

type sinkBankSnapshot struct {
	store    map[string]string
	balances map[string]sdk.Coins
	supply   sdk.Coin
}

func captureSinkBankState(t *testing.T, h *sinkHarness, ctx sdk.Context, addresses ...sdk.AccAddress) sinkBankSnapshot {
	t.Helper()
	bank := h.a.Keepers.Cosmos.Bank
	balances := make(map[string]sdk.Coins)
	for _, addr := range append([]sdk.AccAddress{h.sender, h.bystander, sinkkeeper.Address()}, addresses...) {
		balances[addr.String()] = bank.GetAllBalances(ctx, addr)
	}

	return sinkBankSnapshot{
		store:    storeEntries(t, ctx.KVStore(h.a.GetKey(types.StoreKey)), nil),
		balances: balances,
		supply:   bank.GetSupply(ctx, sdkutil.DenomUakt),
	}
}

func sinkBankMessages(from, to, other sdk.AccAddress) map[string]sdk.Msg {
	return map[string]sdk.Msg{
		"send": banktypes.NewMsgSend(from, to, sdk.NewCoins(coin(10))),
		"multisend": &banktypes.MsgMultiSend{
			Inputs: []banktypes.Input{{
				Address: from.String(),
				Coins:   sdk.NewCoins(coin(10)),
			}},
			Outputs: []banktypes.Output{
				{
					Address: other.String(),
					Coins:   sdk.NewCoins(coin(3)),
				},
				{
					Address: to.String(),
					Coins:   sdk.NewCoins(coin(7)),
				},
			},
		},
	}
}

func TestSinkPublicBankTransfersBlocked(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		for _, name := range []string{"send", "multisend"} {
			t.Run(fmt.Sprintf("%s/enabled=%t", name, enabled), func(t *testing.T) {
				h := newSinkHarness(t, nil)
				res := h.deliver(t, "", h.deposit(10))
				require.Zero(t, res.Code, res.Log)
				p := h.params()
				p.Enabled = enabled
				require.NoError(t, h.update(p))
				bank := h.a.Keepers.Cosmos.Bank
				require.True(t, bank.BlockedAddr(sinkkeeper.Address()))
				other := sdk.AccAddress(h.reporters[0].PubKey().Address())
				res = h.deliver(t, "", sinkBankMessages(h.sender, other, h.bystander)[name])
				require.Zero(t, res.Code, res.Log)
				before := captureSinkBankState(t, h, h.ctx(), other)
				res = h.deliver(t, "", sinkBankMessages(h.sender, sinkkeeper.Address(), h.bystander)[name])
				require.Equal(t, sdkerrors.ErrUnauthorized.ABCICode(), res.Code, res.Log)
				require.Equal(t, sdkerrors.ErrUnauthorized.Codespace(), res.Codespace)
				require.Contains(t, res.Log, sinkkeeper.Address().String()+" is not allowed to receive funds")
				before.balances[h.sender.String()] = before.balances[h.sender.String()].Sub(coin(10))
				require.Equal(t, before, captureSinkBankState(t, h, h.ctx(), other))
				require.Empty(t, sinkEvents(t, res.Events))
				require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
				t.Logf("rejected code=%d codespace=%s log=%s; Sink store, supply, and balances unchanged except the 10 uakt fee", res.Code, res.Codespace, res.Log)
			})
		}
	}
}

func TestSinkAuthzBankTransfersBlocked(t *testing.T) {
	for _, granted := range []bool{false, true} {
		for _, name := range []string{"send", "multisend"} {
			t.Run(fmt.Sprintf("%s/granted=%t", name, granted), func(t *testing.T) {
				h := newSinkHarness(t, nil)
				res := h.deliver(t, "", h.deposit(10))
				require.Zero(t, res.Code, res.Log)
				key := h.key
				if granted {
					key = h.reporters[0]
				}

				grantee := sdk.AccAddress(key.PubKey().Address())
				msg := sinkBankMessages(h.sender, sinkkeeper.Address(), h.bystander)[name]
				var authorization authz.Authorization
				if granted {
					authorization = authz.NewGenericAuthorization(sdk.MsgTypeURL(msg))
					if name == "send" {
						authorization = banktypes.NewSendAuthorization(sdk.NewCoins(coin(10)), nil)
					}

					grant, err := authz.NewMsgGrant(h.sender, grantee, authorization, nil)
					require.NoError(t, err)
					res = h.deliver(t, "", grant)
					require.Zero(t, res.Code, res.Log)
				}

				before := captureSinkBankState(t, h, h.ctx(), grantee)
				exec := authz.NewMsgExec(grantee, []sdk.Msg{msg})
				res = h.deliverBytes(t, h.txFrom(t, key, "", &exec))
				require.Equal(t, sdkerrors.ErrUnauthorized.ABCICode(), res.Code, res.Log)
				require.Contains(t, res.Log, sinkkeeper.Address().String()+" is not allowed to receive funds")
				before.balances[grantee.String()] = before.balances[grantee.String()].Sub(coin(10))
				require.Equal(t, before, captureSinkBankState(t, h, h.ctx(), grantee))
				require.Empty(t, sinkEvents(t, res.Events))
				require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
				if granted {
					retained, _ := h.a.Keepers.Cosmos.Authz.GetAuthorization(h.ctx(), grantee, h.sender, sdk.MsgTypeURL(msg))
					require.Equal(t, authorization, retained)
				}

				t.Logf("authz dispatch rejected code=%d log=%s; grant, Sink store, supply, and transfer balances retained", res.Code, res.Log)
				recipient := sdk.AccAddress(h.reporters[1].PubKey().Address())
				exec = authz.NewMsgExec(grantee, []sdk.Msg{sinkBankMessages(h.sender, recipient, h.bystander)[name]})
				res = h.deliverBytes(t, h.txFrom(t, key, "", &exec))
				require.Zero(t, res.Code, res.Log)
				if granted && name == "send" {
					remaining, _ := h.a.Keepers.Cosmos.Authz.GetAuthorization(h.ctx(), grantee, h.sender, sdk.MsgTypeURL(msg))
					require.Nil(t, remaining)
				}
			})
		}
	}
}

func TestSinkAuthzNestedDepositRejected(t *testing.T) {
	for _, granted := range []bool{false, true} {
		t.Run(fmt.Sprintf("granted=%t", granted), func(t *testing.T) {
			h := newSinkHarness(t, nil)
			res := h.deliver(t, "", h.deposit(10))
			require.Zero(t, res.Code, res.Log)
			key := h.key
			if granted {
				key = h.reporters[0]
			}

			grantee := sdk.AccAddress(key.PubKey().Address())
			msg := h.deposit(10)
			authorization := authz.NewGenericAuthorization(sdk.MsgTypeURL(msg))
			if granted {
				grant, err := authz.NewMsgGrant(h.sender, grantee, authorization, nil)
				require.NoError(t, err)
				res = h.deliver(t, "", grant)
				require.Zero(t, res.Code, res.Log)
			}

			before := captureSinkBankState(t, h, h.ctx(), grantee)
			exec := authz.NewMsgExec(grantee, []sdk.Msg{msg})
			res = h.deliverBytes(t, h.txFrom(t, key, "", &exec))
			require.Equal(t, types.ErrDepositNotSole.ABCICode(), res.Code, res.Log)
			require.Equal(t, types.ErrDepositNotSole.Codespace(), res.Codespace)
			before.balances[grantee.String()] = before.balances[grantee.String()].Sub(coin(10))
			require.Equal(t, before, captureSinkBankState(t, h, h.ctx(), grantee))
			require.Empty(t, sinkEvents(t, res.Events))
			require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
			if granted {
				retained, _ := h.a.Keepers.Cosmos.Authz.GetAuthorization(h.ctx(), grantee, h.sender, sdk.MsgTypeURL(msg))
				require.Equal(t, authorization, retained)
			}

			t.Logf("nested MsgSink rejected code=%d codespace=%s log=%s; no deposit or usage written", res.Code, res.Codespace, res.Log)
			res = h.deliver(t, "", h.deposit(10))
			require.Zero(t, res.Code, res.Log)
			require.Equal(t, uint64(1), h.id(2).Sequence)
		})
	}
}
