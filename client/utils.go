package client

import (
	b64 "encoding/base64"

	"github.com/spf13/pflag"

	errorsmod "cosmossdk.io/errors"

	"github.com/cosmos/cosmos-sdk/client/flags"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/query"
)

// ReadPageRequest reads and builds the necessary page request flags for pagination.
func ReadPageRequest(flagSet *pflag.FlagSet) (*query.PageRequest, error) {
	pageKeyStr, _ := flagSet.GetString(flags.FlagPageKey)
	offset, _ := flagSet.GetUint64(flags.FlagOffset)
	limit, _ := flagSet.GetUint64(flags.FlagLimit)
	countTotal, _ := flagSet.GetBool(flags.FlagCountTotal)
	page, _ := flagSet.GetUint64(flags.FlagPage)
	reverse, _ := flagSet.GetBool(flags.FlagReverse)

	if page > 1 && offset > 0 {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "page and offset cannot be used together")
	}

	// Clear page key if using page numbers (page and key are mutually exclusive)
	if page > 1 {
		offset = (page - 1) * limit
	}

	var pageKey []byte
	if pageKeyStr != "" {
		var err error
		pageKey, err = b64.StdEncoding.DecodeString(pageKeyStr)
		if err != nil {
			return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid pagination key")
		}
	}

	return &query.PageRequest{
		Key:        pageKey,
		Offset:     offset,
		Limit:      limit,
		CountTotal: countTotal,
		Reverse:    reverse,
	}, nil
}
