package sdkutil

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// RenderQueryResponse uses codec to render query response. Returns error incase of failure.
func RenderQueryResponse(cdc *codec.Codec, obj interface{}) ([]byte, error) {
	response, err := codec.MarshalJSONIndent(cdc, obj)
	if err != nil {
		return nil, sdkerrors.New("sdkutil", 1, err.Error())
	}
	return response, nil
}
