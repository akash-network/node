package keeper

import (
	"context"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	bmetypes "pkg.akt.dev/go/node/bme/v1"
)

type BankKeeper interface {
	SpendableCoins(ctx context.Context, addr sdk.AccAddress) sdk.Coins
	SpendableCoin(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
}

type BMEKeeper interface {
	BurnMintFromAddressToModuleAccount(sdk.Context, sdk.AccAddress, string, sdk.Coin, string) (sdk.DecCoin, error)
	BurnMintFromModuleAccountToAddress(sdk.Context, string, sdk.AccAddress, sdk.Coin, string) (sdk.DecCoin, error)
	BurnMintOnAccount(sdk.Context, sdk.AccAddress, sdk.Coin, string) (sdk.DecCoin, error)
	GetMintStatus(sdk.Context) (bmetypes.MintStatus, error)
}

type AuthzKeeper interface {
	DeleteGrant(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, msgType string) error
	GetAuthorization(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, msgType string) (authz.Authorization, *time.Time)
	SaveGrant(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, authorization authz.Authorization, expiration *time.Time) error
	IterateGrants(ctx context.Context, handler func(granterAddr sdk.AccAddress, granteeAddr sdk.AccAddress, grant authz.Grant) bool)
	GetGranteeGrantsByMsgType(ctx context.Context, grantee sdk.AccAddress, msgType string, onGrant authzkeeper.OnGrantFn)
}
