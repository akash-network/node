package types

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/sdkutil"
)

var (
	keyAcc, _ = sdk.AccAddressFromBech32("akash1qtqpdszzakz7ugkey7ka2cmss95z26ygar2mgr")
	//keyParams = sdk.NewKVStoreKey(params.StoreKey)

	errWildcard = errors.New("wildcard string error can't be matched")
)

type testEventParsing struct {
	msg    sdkutil.Event
	expErr error
}

func (tep testEventParsing) testMessageType() func(t *testing.T) {
	_, err := ParseEvent(tep.msg)
	return func(t *testing.T) {
		t.Logf("ERR: %v", err)
		// expected error doesn't match returned    || error returned but not expected
		if (tep.expErr != nil && errors.Is(err, tep.expErr)) || (err != nil && tep.expErr == nil) {
			// if the error expected is errWildcard to catch untyped errors, don't fail the test, the error was expected.
			if errors.Is(tep.expErr, errWildcard) {
				t.Errorf("unexpected error: %v exp: %v", err, tep.expErr)
				t.Logf("%T %v", errors.Cause(err), err)
				t.Logf("%+v", tep)
			}
		}
	}
}

var TEPS = []testEventParsing{
	{
		msg: sdkutil.Event{
			Type: "nil",
		},
		expErr: sdkutil.ErrUnknownType,
	},
	{
		msg: sdkutil.Event{
			Type: sdkutil.EventTypeMessage,
		},
		expErr: sdkutil.ErrUnknownModule,
	},

	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
		},
		expErr: sdkutil.ErrUnknownAction,
	},
	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: "nil",
		},
		expErr: sdkutil.ErrUnknownModule,
	},

	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: "nil",
		},
		expErr: sdkutil.ErrUnknownAction,
	},

	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionDeploymentCreate,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
			},
		},
		expErr: nil,
	},
	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionDeploymentCreate,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "abc",
				},
			},
		},
		expErr: errWildcard,
	},
	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionDeploymentCreate,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
			},
		},
		expErr: errWildcard,
	},

	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionDeploymentUpdate,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
			},
		},
		expErr: nil,
	},
	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionDeploymentUpdate,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: "neh",
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
			},
		},
		expErr: errWildcard,
	},
	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionDeploymentUpdate,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
			},
		},
		expErr: errWildcard,
	},
}

func TestEventParsing(t *testing.T) {
	for i, test := range TEPS {
		t.Run(fmt.Sprintf("%d", i),
			test.testMessageType())
	}
}
