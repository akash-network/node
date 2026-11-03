package keeper

import (
	"context"
	"math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/sink/v1"
)

type sweepContextKey struct{}

type sweepAuthorization struct {
	recipient sdk.AccAddress
	amount    sdk.Coins
}

type sinkRequestContextKey struct{}

type depositContextKey struct{}

type refundContextKey struct{}

type sinkRequest struct {
	sender   string
	destType types.DestType
	dest     string
	denom    string
	amount   string
}

type depositAuthorization struct {
	sender sdk.AccAddress
	amount sdk.Coins
}

type refundAuthorization struct {
	recipient sdk.AccAddress
	amount    sdk.Coins
}

// SendRestriction guards every bank path into and out of escrow. It never
// creates records; the MsgSink handler owns the complete atomic deposit.
func (k Keeper) SendRestriction(goCtx context.Context, from, to sdk.AccAddress, coins sdk.Coins) (sdk.AccAddress, error) {
	if from.Equals(sinkAddress) {
		ctx := sdk.UnwrapSDKContext(goCtx)
		if refund, ok := ctx.Value(refundContextKey{}).(refundAuthorization); ok && to.Equals(refund.recipient) && coins.Equal(refund.amount) {
			return to, nil
		}

		if sweep, ok := ctx.Value(sweepContextKey{}).(sweepAuthorization); ok && to.Equals(sweep.recipient) && coins.Equal(sweep.amount) {
			initialized, err := k.initialized(ctx)
			if err != nil {
				return nil, err
			}

			if !initialized {
				return to, nil
			}
		}

		return nil, sdkerrors.ErrUnauthorized.Wrap("sink escrow cannot be sent")
	}

	if !to.Equals(sinkAddress) {
		return to, nil
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	d, ok := ctx.Value(depositContextKey{}).(depositAuthorization)
	if !ok || !from.Equals(d.sender) || !coins.Equal(d.amount) {
		return nil, sdkerrors.ErrUnauthorized.Wrap("sink deposits require MsgSink")
	}

	return to, nil
}

// Deposit validates and escrows one deposit, writes its immutable proof record
// and returns that record's ID.
func (k Keeper) Deposit(ctx sdk.Context, msg *types.MsgSink) (types.ID, error) {
	if msg == nil {
		return types.ID{}, sdkerrors.ErrInvalidRequest.Wrap("nil request")
	}

	if err := msg.ValidateBasic(); err != nil {
		return types.ID{}, err
	}

	cache, write := ctx.CacheContext()

	// Simulation uses committed state after the transient allocator has been cleared.
	// Its proposed transfer belongs to the next block, and BaseApp discards the writes.
	if cache.ExecMode() == sdk.ExecModeSimulate {
		if cache.BlockHeight() == math.MaxInt64 {
			return types.ID{}, types.ErrOverflow.Wrap("simulation height")
		}

		cache = cache.WithBlockHeight(max(int64(1), cache.BlockHeight()+1))
	}

	request, ok := cache.Value(sinkRequestContextKey{}).(sinkRequest)
	if !ok || request.sender != msg.Sender || request.destType != msg.DestType || request.dest != msg.Dest || request.denom != msg.Amount.Denom || request.amount != msg.Amount.Amount.String() {
		return types.ID{}, types.ErrDepositNotSole
	}

	sender, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return types.ID{}, err
	}

	acc := k.accounts.GetAccount(cache, sender)
	if _, module := acc.(sdk.ModuleAccountI); module || acc == nil || acc.GetPubKey() == nil {
		return types.ID{}, types.ErrSenderNotSigningAccount
	}

	p, err := k.params.Get(cache)
	if err != nil {
		return types.ID{}, err
	}

	if !p.DepositsEnabled {
		return types.ID{}, types.ErrDepositsDisabled
	}

	policy, err := k.currentPolicy(cache)
	if err != nil {
		return types.ID{}, err
	}

	if policy.Threshold == 0 {
		return types.ID{}, types.ErrInvalidReporters
	}

	vaults := make([]types.SolanaAddress, 0, len(p.AllowedDestinationVaults))
	for _, v := range p.AllowedDestinationVaults {
		a, err := types.ParseSolanaAddress(v)
		if err != nil {
			return types.ID{}, err
		}

		vaults = append(vaults, a)
	}

	if _, err = types.ParseDestination(msg.Dest, vaults); err != nil {
		return types.ID{}, err
	}

	if msg.Amount.IsLT(p.MinAmountPerTransfer) {
		return types.ID{}, types.ErrBelowMinTransfer
	}

	if msg.Amount.IsGT(p.MaxAmountPerTransfer) {
		return types.ID{}, types.ErrAboveMaxTransfer
	}

	epoch, err := types.EpochOf(cache.BlockTime())
	if err != nil {
		return types.ID{}, err
	}

	u, err := k.GetAccountUsage(cache, sender)
	if err != nil {
		return types.ID{}, err
	}

	u, err = u.At(epoch)
	if err != nil {
		return types.ID{}, err
	}

	send, err := k.sendUsage.Get(cache)
	if err != nil {
		return types.ID{}, err
	}

	send, err = send.At(epoch)
	if err != nil {
		return types.ID{}, err
	}

	nextAccount, err := types.AddAmount(u.EpochSent, msg.Amount)
	if err != nil {
		return types.ID{}, err
	}

	if nextAccount.IsGT(p.MaxEpochAmountPerAccount) {
		return types.ID{}, types.ErrAccountDailyAmountExceeded
	}

	nextSend, err := types.AddAmount(send.Amount, msg.Amount)
	if err != nil {
		return types.ID{}, err
	}

	if nextSend.IsGT(p.MaxTransferAmountPerEpoch) {
		return types.ID{}, types.ErrSendCapReached
	}

	id, err := k.nextID(cache, sender)
	if err != nil {
		return types.ID{}, err
	}

	exists, err := k.transfers.Has(cache, id)
	if err != nil {
		return types.ID{}, err
	}

	if exists {
		return types.ID{}, types.ErrTransferExists
	}

	if err = k.CheckBalance(cache); err != nil {
		return types.ID{}, err
	}

	coins := sdk.NewCoins(msg.Amount)
	sendCtx := cache.WithValue(depositContextKey{}, depositAuthorization{sender: sender, amount: coins})
	if err = k.bank.SendCoinsFromAccountToModule(sendCtx, sender, types.ModuleName, coins); err != nil {
		return types.ID{}, err
	}

	if err = k.transfers.Set(cache, id, types.RecordData{Dest: msg.Dest, Amount: msg.Amount}); err != nil {
		return types.ID{}, err
	}

	u.TotalSent, err = types.AddAmount(u.TotalSent, msg.Amount)
	if err != nil {
		return types.ID{}, err
	}

	u.EpochSent = nextAccount
	send.Amount = nextSend

	if err = k.usage.Set(cache, sender, u); err != nil {
		return types.ID{}, err
	}

	if err = k.sendUsage.Set(cache, send); err != nil {
		return types.ID{}, err
	}

	if err = k.put(cache, id, msg.Amount, nil, types.SettlementData{
		Status:        types.StatusTransferPending,
		Round:         1,
		PolicyVersion: policy.Version,
	}); err != nil {
		return types.ID{}, err
	}

	if err = k.CheckBalance(cache); err != nil {
		return types.ID{}, err
	}

	if err = cache.EventManager().EmitTypedEvent(&types.EventTransferCreated{
		ID:        id,
		Sender:    msg.Sender,
		Recipient: msg.Dest,
		Amount:    msg.Amount,
		Height:    id.Height,
	}); err != nil {
		return types.ID{}, err
	}

	write()

	return id, nil
}
