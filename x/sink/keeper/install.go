package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

// CommunityPool receives coins found at the sink address before installation.
type CommunityPool interface {
	FundCommunityPool(ctx context.Context, amount sdk.Coins, sender sdk.AccAddress) error
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
// to the community pool, keeping reserve + pending + minted + voided equal to
// the sink balance. It returns the moved coins.
func (k Keeper) PrepareInstall(ctx sdk.Context, pool CommunityPool) (sdk.Coins, error) {
	if ctx.KVStore(k.key).Has(paramsKey) {
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
	write()
	return balance, nil
}
