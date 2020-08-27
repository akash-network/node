package sdkutil

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	// ErrCouldNotRenderObject indicates response rendering error.
	ErrCouldNotRenderObject = sdkerrors.New("sdkutil", 1, "could not render object")
)

// RenderQueryResponse uses codec to render query response. Returns error incase of failure.
func RenderQueryResponse(cdc *codec.LegacyAmino, obj interface{}) ([]byte, error) {
	response, err := codec.MarshalJSONIndent(cdc, obj)
	if err != nil {
		return nil, sdkerrors.Wrap(ErrCouldNotRenderObject, err.Error())
	}
	return response, nil
}
