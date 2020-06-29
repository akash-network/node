package sdkutil

import (
	"errors"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	akashEventMessageV1 = "akash.v1"

	// EventTypeMessage defines the Akash message string
	EventTypeMessage = akashEventMessageV1
)

var (
	// ErrNotFound is the error with message "Not found"
	ErrNotFound = errors.New("Not found")
	// ErrUnknownType is the error with message "Unknown type"
	ErrUnknownType = errors.New("Unknown type")
	// ErrUnknownModule is the error with message "Unknown module"
	ErrUnknownModule = errors.New("Unknown module")
	// ErrUnknownAction is the error with message "Unknown action"
	ErrUnknownAction = errors.New("Unknown action")
)

type BaseModuleEvent struct {
	Module string `json:"module"`
	Action string `json:"action"`
}

type ModuleEvent interface {
	ToSDKEvent() sdk.Event
}

// Event stores type, module, action and attributes list of sdk
type Event struct {
	Type       string
	Module     string
	Action     string
	Attributes []sdk.Attribute
}

// ParseEvent parses string to event
func ParseEvent(sev sdk.StringEvent) (Event, error) {
	ev := Event{Type: sev.Type, Attributes: sev.Attributes}
	var err error

	if ev.Module, err = GetString(sev.Attributes, sdk.AttributeKeyModule); err != nil {
		return ev, err
	}

	if ev.Action, err = GetString(sev.Attributes, sdk.AttributeKeyAction); err != nil {
		return ev, err
	}

	return ev, nil
}

// GetUint64 take sdk attributes, key and returns uint64 value. Returns error incase of failure.
func GetUint64(attrs []sdk.Attribute, key string) (uint64, error) {
	sval, err := GetString(attrs, key)
	if err != nil {
		return 0, err
	}
	val, err := strconv.ParseUint(sval, 10, 64)
	return val, err
}

// GetAccAddress take sdk attributes, key and returns account address. Returns error incase of failure.
func GetAccAddress(attrs []sdk.Attribute, key string) (sdk.AccAddress, error) {
	sval, err := GetString(attrs, key)
	if err != nil {
		return nil, err
	}
	val, err := sdk.AccAddressFromBech32(sval)
	return val, err
}

// GetString take sdk attributes, key and returns key value. Returns error incase of failure.
func GetString(attrs []sdk.Attribute, key string) (string, error) {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value, nil
		}
	}
	return "", ErrNotFound
}
