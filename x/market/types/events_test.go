package types

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/pkg/errors"

	"github.com/ovrclk/akash/sdkutil"
	"github.com/stretchr/testify/require"
)

var (
	// converting string to address
	accBytes, _ = sdk.GetFromBech32("akash1qtqpdszzakz7ugkey7ka2cmss95z26ygar2mgr", "akash")
	keyAcc      = sdk.AccAddress(accBytes)
	//keyParams = sdk.NewKVStoreKey(params.StoreKey)

	errWildcard = errors.New("wildcard string error can't be matched")
	evOwnerKey  = "owner"
	evDSeqKey   = "dseq"
	evGSeqKey   = "gseq"
)

type testEventParsing struct {
	msg    sdkutil.Event
	expErr error
}

func (tep testEventParsing) testMessageType() func(t *testing.T) {
	_, err := ParseEvent(tep.msg)
	return func(t *testing.T) {
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
			Action: evActionOrderCreated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
				{
					Key:   evGSeqKey,
					Value: "2",
				},
				{
					Key:   evOSeqKey,
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
			Action: evActionOrderCreated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: "nooo",
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
				{
					Key:   evGSeqKey,
					Value: "2",
				},
				{
					Key:   evOSeqKey,
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
			Action: evActionOrderCreated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
				{
					Key:   evGSeqKey,
					Value: "2",
				},
				{
					Key:   evOSeqKey,
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
			Action: evActionOrderClosed,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
				{
					Key:   evGSeqKey,
					Value: "2",
				},
				{
					Key:   evOSeqKey,
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
			Action: evActionBidCreated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
				{
					Key:   evGSeqKey,
					Value: "2",
				},
				{
					Key:   evOSeqKey,
					Value: "5",
				},
				{
					Key:   evProviderKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evPriceDenomKey,
					Value: "akt",
				},
				{
					Key:   evPriceAmountKey,
					Value: "23",
				},
			},
		},
		expErr: nil,
	},
	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionBidCreated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
				{
					Key:   evGSeqKey,
					Value: "2",
				},
				{
					Key:   evOSeqKey,
					Value: "5",
				},
				{
					Key:   evProviderKey,
					Value: "yesss",
				},
				{
					Key:   evPriceDenomKey,
					Value: "akt",
				},
				{
					Key:   evPriceAmountKey,
					Value: "23",
				},
			},
		},
		expErr: errWildcard,
	},
	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionBidCreated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
				{
					Key:   evGSeqKey,
					Value: "2",
				},
				{
					Key:   evOSeqKey,
					Value: "5",
				},
				{
					Key:   evProviderKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evPriceDenomKey,
					Value: "akt",
				},
				{
					Key:   evPriceAmountKey,
					Value: "hello",
				},
			},
		},
		expErr: errWildcard,
	},

	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionBidClosed,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
				{
					Key:   evGSeqKey,
					Value: "2",
				},
				{
					Key:   evOSeqKey,
					Value: "5",
				},
				{
					Key:   evProviderKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evPriceDenomKey,
					Value: "akt",
				},
				{
					Key:   evPriceAmountKey,
					Value: "23",
				},
			},
		},
		expErr: nil,
	},

	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionLeaseCreated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
				{
					Key:   evGSeqKey,
					Value: "2",
				},
				{
					Key:   evOSeqKey,
					Value: "5",
				},
				{
					Key:   evProviderKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evPriceDenomKey,
					Value: "akt",
				},
				{
					Key:   evPriceAmountKey,
					Value: "23",
				},
			},
		},
		expErr: nil,
	},
	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionLeaseCreated,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
				{
					Key:   evGSeqKey,
					Value: "2",
				},
				{
					Key:   evOSeqKey,
					Value: "5",
				},
				{
					Key:   evProviderKey,
					Value: "hello",
				},
				{
					Key:   evPriceDenomKey,
					Value: "akt",
				},
				{
					Key:   evPriceAmountKey,
					Value: "23",
				},
			},
		},
		expErr: errWildcard,
	},

	{
		msg: sdkutil.Event{
			Type:   sdkutil.EventTypeMessage,
			Module: ModuleName,
			Action: evActionLeaseClosed,
			Attributes: []sdk.Attribute{
				{
					Key:   evOwnerKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evDSeqKey,
					Value: "5",
				},
				{
					Key:   evGSeqKey,
					Value: "2",
				},
				{
					Key:   evOSeqKey,
					Value: "5",
				},
				{
					Key:   evProviderKey,
					Value: keyAcc.String(),
				},
				{
					Key:   evPriceDenomKey,
					Value: "akt",
				},
				{
					Key:   evPriceAmountKey,
					Value: "23",
				},
			},
		},
		expErr: nil,
	},
}

func TestEventParsing(t *testing.T) {
	for i, test := range TEPS {
		t.Run(fmt.Sprintf("%d", i),
			test.testMessageType())
	}
}
