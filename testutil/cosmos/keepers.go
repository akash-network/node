package keeper

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

//go:generate mockery --name BankKeeper --output ./mocks
type BankKeeper interface {
	SendCoinsFromModuleToAccount(ctx sdk.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx sdk.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx sdk.Context, senderModule string, recipientModule string, amt sdk.Coins) error
}

//go:generate mockery --name TakeKeeper --output ./mocks
type TakeKeeper interface {
	SubtractFees(ctx sdk.Context, amt sdk.Coin) (sdk.Coin, sdk.Coin, error)
}

//go:generate mockery --name DistrKeeper --output ./mocks
type DistrKeeper interface {
	GetFeePool(ctx sdk.Context) distrtypes.FeePool
	SetFeePool(ctx sdk.Context, pool distrtypes.FeePool)
}

//go:generate mockery --name AuthzKeeper --output ./mocks
type AuthzKeeper interface {
	DeleteGrant(ctx sdk.Context, grantee, granter sdk.AccAddress, msgType string) error
	GetAuthorization(ctx sdk.Context, grantee, granter sdk.AccAddress, msgType string) (cap authz.Authorization, expiration *time.Time)
	SaveGrant(ctx sdk.Context, grantee, granter sdk.AccAddress, authorization authz.Authorization, expiration *time.Time) error
	IterateGrants(ctx sdk.Context, handler func(granterAddr sdk.AccAddress, granteeAddr sdk.AccAddress, grant authz.Grant) bool)
}
