package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

// Distribution receives coins found at the sink address before installation
// and holds the withdraw addresses that may point at it.
type Distribution interface {
	FundCommunityPool(ctx context.Context, amount sdk.Coins, sender sdk.AccAddress) error
	IterateDelegatorWithdrawAddrs(ctx context.Context, handler func(del, addr sdk.AccAddress) (stop bool))
	DeleteDelegatorWithdrawAddr(ctx context.Context, delAddr, withdrawAddr sdk.AccAddress) error
}

type sweepContextKey struct{}
type sweepAuthorization struct {
	recipient sdk.AccAddress
	amount    sdk.Coins
}

// PrepareInstall readies the deterministic sink address for an upgrade that
// adds this module, before migrations run sink InitGenesis. Anyone can send
// coins to the address or create an account there before the upgrade height,
// so neither may fail the upgrade. A non-module account becomes the sink
// module account with the same account number. Its whole balance then moves
// to the community pool, keeping pending + minted + voided equal to the sink
// balance. Withdraw addresses set to the sink address, which only
// becomes a blocked address with this module, are cleared so rewards go to
// the delegators themselves. It returns the moved coins.
func (k Keeper) PrepareInstall(ctx sdk.Context, pool Distribution) (sdk.Coins, error) {
	if k.initialized(ctx) {
		return nil, fmt.Errorf("sink already initialized")
	}
	addr := Address()
	cache, write := ctx.CacheContext()
	switch acc := k.accounts.GetAccount(cache, addr).(type) {
	case nil:
		// InitGenesis creates the module account.
	case sdk.ModuleAccountI:
		if acc.GetName() != types.ModuleName {
			return nil, fmt.Errorf("sink address belongs to module %q", acc.GetName())
		}
	default:
		// Vesting schedules and any other account state are dropped; a module
		// account has no key, so nothing could have signed for it.
		base := authtypes.NewBaseAccount(addr, nil, acc.GetAccountNumber(), acc.GetSequence())
		k.accounts.SetAccount(cache, authtypes.NewModuleAccount(base, types.ModuleName))
	}
	balance := k.bank.GetAllBalances(cache, addr)
	if !balance.IsZero() {
		sweep := cache.WithValue(sweepContextKey{}, sweepAuthorization{
			recipient: authtypes.NewModuleAddress(distrtypes.ModuleName), amount: balance,
		})
		if err := pool.FundCommunityPool(sweep, balance, addr); err != nil {
			return nil, err
		}
		if !k.bank.GetAllBalances(cache, addr).IsZero() {
			return nil, fmt.Errorf("sink address balance was not cleared")
		}
		cache.EventManager().EmitEvent(sdk.NewEvent("sink_preexisting_balance",
			sdk.NewAttribute("amount", balance.String()), sdk.NewAttribute("recipient", "community_pool")))
	}
	// A payment to a blocked address fails, which would break reward
	// withdrawals, undelegations and zero-height export. Collect first, then
	// delete, rather than deleting while iterating.
	var delegators []sdk.AccAddress
	pool.IterateDelegatorWithdrawAddrs(cache, func(del, withdraw sdk.AccAddress) bool {
		if withdraw.Equals(addr) {
			delegators = append(delegators, del)
		}
		return false
	})
	for _, del := range delegators {
		if err := pool.DeleteDelegatorWithdrawAddr(cache, del, addr); err != nil {
			return nil, err
		}
		cache.EventManager().EmitEvent(sdk.NewEvent("sink_withdraw_address_reset", sdk.NewAttribute("delegator", del.String())))
	}
	write()
	return balance, nil
}
