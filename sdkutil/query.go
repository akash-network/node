package sdkutil

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func RenderQueryResponse(cdc *codec.Codec, obj interface{}) ([]byte, sdk.Error) {
	response, err := codec.MarshalJSONIndent(cdc, obj)
	if err != nil {
		return nil, sdk.ErrInternal("error marshaling response: " + err.Error())
	}
	return response, nil
}
