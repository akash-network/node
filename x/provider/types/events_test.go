package types

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

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
		t.Logf("%+v", tep)
		// if the error expected is errWildcard to catch untyped errors, don't fail the test, the error was expected.
		if errors.Is(tep.expErr, errWildcard) {
			require.Error(t, err)
		} else {
			require.Equal(t, tep.expErr, err)
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
			Action: evActionProviderCreated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
			},
		},
		expErr: nil,
	},
	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionProviderCreated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: "hello",
				},
			},
		},
		expErr: errWildcard,
	},
	{
		msg: sdkutil.Event{
			Type:       sdkutil.EventTypeMessage,
			Module:     ModuleName,
			Action:     evActionProviderCreated,
			Attributes: []sdk.Attribute{},
		},
		expErr: errWildcard,
	},

	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionProviderUpdated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
			},
		},
		expErr: nil,
	},
	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionProviderUpdated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: "hello",
				},
			},
		},
		expErr: errWildcard,
	},
	{
		msg: sdkutil.Event{
			Type:       sdkutil.EventTypeMessage,
			Module:     ModuleName,
			Action:     evActionProviderUpdated,
			Attributes: []sdk.Attribute{},
		},
		expErr: errWildcard,
	},

	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionProviderDeleted,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
			},
		},
		expErr: nil,
	},
	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionProviderDeleted,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: "hello",
				},
			},
		},
		expErr: errWildcard,
	},
	{
		msg: sdkutil.Event{
			Type:       sdkutil.EventTypeMessage,
			Module:     ModuleName,
			Action:     evActionProviderDeleted,
			Attributes: []sdk.Attribute{},
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
