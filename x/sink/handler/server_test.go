package handler

import (
	"bytes"
	"context"
	"errors"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/x/sink/keeper"
)

type commandKeeper struct {
	keeper.Keeper
	authority  string
	policy     types.ReporterPolicy
	params     types.Params
	readErr    error
	commandErr error
	calls      []sdk.Msg
	id         types.ID
}

func (k *commandKeeper) GetAuthority() string { return k.authority }

func (k *commandKeeper) GetCurrentPolicy(context.Context) (types.ReporterPolicy, error) {
	return k.policy, k.readErr
}

func (k *commandKeeper) GetParams(context.Context) (types.Params, error) { return k.params, k.readErr }

func (k *commandKeeper) Deposit(_ sdk.Context, msg *types.MsgSink) (types.ID, error) {
	k.calls = append(k.calls, msg)
	return k.id, k.commandErr
}

func (k *commandKeeper) UpdateParams(_ sdk.Context, msg *types.MsgUpdateParams) error {
	k.calls = append(k.calls, msg)
	return k.commandErr
}

func (k *commandKeeper) RequestVoid(_ sdk.Context, msg *types.MsgRequestVoid) error {
	k.calls = append(k.calls, msg)
	return k.commandErr
}

func (k *commandKeeper) ReportOutcome(_ sdk.Context, msg *types.MsgReportOutcome) error {
	k.calls = append(k.calls, msg)
	return k.commandErr
}

func (k *commandKeeper) VetoVoid(_ sdk.Context, msg *types.MsgVetoVoid) error {
	k.calls = append(k.calls, msg)
	return k.commandErr
}

func (k *commandKeeper) Resolve(_ sdk.Context, msg types.Resolution) error {
	k.calls = append(k.calls, msg)
	return k.commandErr
}

func (k *commandKeeper) FinalizeRefund(_ sdk.Context, msg *types.MsgFinalizeRefund) error {
	k.calls = append(k.calls, msg)
	return k.commandErr
}

func (k *commandKeeper) GuardianPause(_ sdk.Context, msg *types.MsgGuardianPause) error {
	k.calls = append(k.calls, msg)
	return k.commandErr
}

func dispatch(server types.MsgServer, ctx context.Context, msg sdk.Msg) (any, error) {
	switch m := msg.(type) {
	case *types.MsgSink:
		return server.Sink(ctx, m)
	case *types.MsgUpdateParams:
		return server.UpdateParams(ctx, m)
	case *types.MsgRequestVoid:
		return server.RequestVoid(ctx, m)
	case *types.MsgReportOutcome:
		return server.ReportOutcome(ctx, m)
	case *types.MsgVetoVoid:
		return server.VetoVoid(ctx, m)
	case *types.MsgRestartTransfer:
		return server.RestartTransfer(ctx, m)
	case *types.MsgConfirmMint:
		return server.ConfirmMint(ctx, m)
	case *types.MsgConfirmVoid:
		return server.ConfirmVoid(ctx, m)
	case *types.MsgFinalizeRefund:
		return server.FinalizeRefund(ctx, m)
	case *types.MsgGuardianPause:
		return server.GuardianPause(ctx, m)
	default:
		panic("unhandled message")
	}
}

func TestMessagesValidateBeforeKeeper(t *testing.T) {
	for _, tc := range []struct {
		name           string
		empty, missing sdk.Msg
	}{
		{"Sink", &types.MsgSink{}, (*types.MsgSink)(nil)},
		{"UpdateParams", &types.MsgUpdateParams{}, (*types.MsgUpdateParams)(nil)},
		{"RequestVoid", &types.MsgRequestVoid{}, (*types.MsgRequestVoid)(nil)},
		{"ReportOutcome", &types.MsgReportOutcome{}, (*types.MsgReportOutcome)(nil)},
		{"VetoVoid", &types.MsgVetoVoid{}, (*types.MsgVetoVoid)(nil)},
		{"RestartTransfer", &types.MsgRestartTransfer{}, (*types.MsgRestartTransfer)(nil)},
		{"ConfirmMint", &types.MsgConfirmMint{}, (*types.MsgConfirmMint)(nil)},
		{"ConfirmVoid", &types.MsgConfirmVoid{}, (*types.MsgConfirmVoid)(nil)},
		{"FinalizeRefund", &types.MsgFinalizeRefund{}, (*types.MsgFinalizeRefund)(nil)},
		{"GuardianPause", &types.MsgGuardianPause{}, (*types.MsgGuardianPause)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("nil", func(t *testing.T) {
				require.NotPanics(t, func() {
					response, err := dispatch(NewServer(nil), context.Background(), tc.missing)
					require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)
					require.Nil(t, response)
				})
			})

			t.Run("invalid", func(t *testing.T) {
				expected := tc.empty.(sdk.HasValidateBasic).ValidateBasic()
				require.Error(t, expected)
				require.NotPanics(t, func() {
					response, err := dispatch(NewServer(nil), context.Background(), tc.empty)
					require.EqualError(t, err, expected.Error())
					require.Nil(t, response)
				})
			})
		})
	}
}

func TestMessageAuthorizationAndDispatch(t *testing.T) {
	authority := sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String()
	reporter := sdk.AccAddress(bytes.Repeat([]byte{2}, 20)).String()
	guardian := sdk.AccAddress(bytes.Repeat([]byte{3}, 20)).String()
	stranger := sdk.AccAddress(bytes.Repeat([]byte{4}, 20)).String()
	id := types.ID{
		Owner:    stranger,
		Height:   10,
		Sequence: 1,
	}

	evidence := types.SolanaEvidence{
		Slot:                 10,
		TransactionSignature: bytes.Repeat([]byte{1}, 64),
	}

	params := types.DefaultParams()
	params.PauseGuardian = guardian
	policy := types.ReporterPolicy{
		Version:   2,
		Members:   []string{reporter},
		Threshold: 1,
	}

	ctx := sdk.Context{}.WithContext(context.Background())

	for _, tc := range []struct {
		name    string
		makeMsg func(string) sdk.Msg
		allowed []string
		denied  error
		reads   bool
	}{
		{
			name: "Sink",
			makeMsg: func(signer string) sdk.Msg {
				return &types.MsgSink{
					Sender:   signer,
					DestType: types.DestTypeSolana,
					Dest:     types.SolanaAddress{1}.String(),
					Amount:   sdk.NewInt64Coin(sdkutil.DenomUakt, 10),
				}
			},
			allowed: []string{stranger},
		},
		{
			name: "UpdateParams",
			makeMsg: func(signer string) sdk.Msg {
				return &types.MsgUpdateParams{
					Authority: signer,
					Params:    params,
				}
			},
			allowed: []string{authority},
			denied:  sdkerrors.ErrUnauthorized,
		},
		{
			name: "RequestVoid",
			makeMsg: func(signer string) sdk.Msg {
				return &types.MsgRequestVoid{
					Authority:     signer,
					ID:            id,
					ExpectedRound: 1,
				}
			},
			allowed: []string{authority},
			denied:  sdkerrors.ErrUnauthorized,
		},
		{
			name: "ReportOutcome",
			makeMsg: func(signer string) sdk.Msg {
				return &types.MsgReportOutcome{
					Reporter:      signer,
					ID:            id,
					PolicyVersion: policy.Version,
					Round:         1,
					Observation: types.Observation{
						Outcome:              types.OutcomeMinted,
						Slot:                 evidence.Slot,
						TransactionSignature: evidence.TransactionSignature,
					},
				}
			},
			allowed: []string{reporter},
			denied:  types.ErrNotReporter,
			reads:   true,
		},
		{
			name: "VetoVoid",
			makeMsg: func(signer string) sdk.Msg {
				return &types.MsgVetoVoid{
					Signer: signer,
					ID:     id,
					Reason: "veto",
				}
			},
			allowed: []string{reporter, authority},
			denied:  types.ErrNotReporter,
			reads:   true,
		},
		{
			name: "RestartTransfer",
			makeMsg: func(signer string) sdk.Msg {
				return &types.MsgRestartTransfer{
					Authority:     signer,
					ID:            id,
					ExpectedRound: 1,
					Reason:        "restart",
				}
			},
			allowed: []string{authority},
			denied:  sdkerrors.ErrUnauthorized,
		},
		{
			name: "ConfirmMint",
			makeMsg: func(signer string) sdk.Msg {
				return &types.MsgConfirmMint{
					Authority:     signer,
					ID:            id,
					ExpectedRound: 1,
					Reason:        "mint",
					Evidence:      evidence,
				}
			},
			allowed: []string{authority},
			denied:  sdkerrors.ErrUnauthorized,
		},
		{
			name: "ConfirmVoid",
			makeMsg: func(signer string) sdk.Msg {
				return &types.MsgConfirmVoid{
					Authority:     signer,
					ID:            id,
					ExpectedRound: 1,
					Reason:        "void",
					Evidence:      evidence,
				}
			},
			allowed: []string{authority},
			denied:  sdkerrors.ErrUnauthorized,
		},
		{
			name: "FinalizeRefund",
			makeMsg: func(signer string) sdk.Msg {
				return &types.MsgFinalizeRefund{
					Executor: signer,
					ID:       id,
				}
			},
			allowed: []string{stranger},
		},
		{
			name: "GuardianPause",
			makeMsg: func(signer string) sdk.Msg {
				return &types.MsgGuardianPause{
					Guardian: signer,
					Deposits: true,
				}
			},
			allowed: []string{guardian},
			denied:  types.ErrNotGuardian,
			reads:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newKeeper := func() *commandKeeper {
				return &commandKeeper{
					authority: authority,
					policy:    policy,
					params:    params,
					id:        id,
				}
			}
			for _, signer := range tc.allowed {
				for _, commandErr := range []error{nil, errors.New("command failed")} {
					k := newKeeper()
					k.commandErr = commandErr
					msg := tc.makeMsg(signer)
					require.NoError(t, msg.(sdk.HasValidateBasic).ValidateBasic())
					response, err := dispatch(NewServer(k), ctx, msg)
					require.ErrorIs(t, err, commandErr)
					require.Equal(t, []sdk.Msg{msg}, k.calls)

					if commandErr != nil {
						require.Nil(t, response)
					} else {
						require.NotNil(t, response)

						if tc.name == "Sink" {
							require.Equal(t, id, response.(*types.MsgSinkResponse).ID)
						}
					}
				}
			}

			if tc.denied != nil {
				k := newKeeper()
				response, err := dispatch(NewServer(k), ctx, tc.makeMsg(stranger))
				require.ErrorIs(t, err, tc.denied)
				require.Nil(t, response)
				require.Empty(t, k.calls)
			}

			if tc.reads {
				k := newKeeper()
				k.readErr = errors.New("authorization read failed")
				response, err := dispatch(NewServer(k), ctx, tc.makeMsg(tc.allowed[0]))
				require.ErrorIs(t, err, k.readErr)
				require.Nil(t, response)
				require.Empty(t, k.calls)
			}
		})
	}
}

func TestGuardianMustBeConfigured(t *testing.T) {
	guardian := sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String()
	k := &commandKeeper{}
	response, err := NewServer(k).GuardianPause(sdk.Context{}.WithContext(context.Background()), &types.MsgGuardianPause{
		Guardian: guardian,
		Deposits: true,
	})
	require.ErrorIs(t, err, types.ErrNotGuardian)
	require.Nil(t, response)
	require.Empty(t, k.calls)
}
