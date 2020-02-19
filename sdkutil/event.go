package sdkutil

import (
	"errors"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	ErrNotFound      = errors.New("Not found")
	ErrUnknownType   = errors.New("Unknown type")
	ErrUnknownModule = errors.New("Unknown module")
	ErrUnknownAction = errors.New("Unknown action")
)

type Event struct {
	Type       string
	Module     string
	Action     string
	Attributes []sdk.Attribute
}

func ParseEvent(sev sdk.StringEvent) (Event, error) {
	ev := Event{Type: sev.Type}
	var err error

	if ev.Module, err = GetString(sev.Attributes, sdk.AttributeKeyModule); err != nil {
		return ev, err
	}

	if ev.Action, err = GetString(sev.Attributes, sdk.AttributeKeyAction); err != nil {
		return ev, err
	}

	return ev, nil
}

func GetUint64(attrs []sdk.Attribute, key string) (uint64, error) {
	sval, err := GetString(attrs, key)
	if err != nil {
		return 0, err
	}
	val, err := strconv.ParseUint(sval, 10, 64)
	return val, err
}

func GetAccAddress(attrs []sdk.Attribute, key string) (sdk.AccAddress, error) {
	sval, err := GetString(attrs, key)
	if err != nil {
		return nil, err
	}
	val, err := sdk.AccAddressFromBech32(sval)
	return val, err
}

func GetString(attrs []sdk.Attribute, key string) (string, error) {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value, nil
		}
	}
	return "", ErrNotFound
}
