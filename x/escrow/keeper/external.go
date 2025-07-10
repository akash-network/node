package keeper

import (
	"context"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
)

type BankKeeper interface {
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
}

type TakeKeeper interface {
	SubtractFees(ctx sdk.Context, amt sdk.Coin) (sdk.Coin, sdk.Coin, error)
}

type AuthzKeeper interface {
	DeleteGrant(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, msgType string) error
	GetAuthorization(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, msgType string) (authz.Authorization, *time.Time)
	SaveGrant(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, authorization authz.Authorization, expiration *time.Time) error
	IterateGrants(ctx context.Context, handler func(granterAddr sdk.AccAddress, granteeAddr sdk.AccAddress, grant authz.Grant) bool)
}
