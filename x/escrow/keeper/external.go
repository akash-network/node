package keeper

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
)

type BankKeeper interface {
	SendCoinsFromModuleToAccount(ctx sdk.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx sdk.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
}

type AuthzKeeper interface {
	DeleteGrant(ctx sdk.Context, grantee, granter sdk.AccAddress, msgType string) error
	GetCleanAuthorization(ctx sdk.Context, grantee, granter sdk.AccAddress, msgType string) (cap authz.Authorization, expiration time.Time)
	SaveGrant(ctx sdk.Context, grantee, granter sdk.AccAddress, authorization authz.Authorization, expiration time.Time) error
}
