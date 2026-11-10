package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

// Deposit validates and escrows one deposit, writes its immutable proof record
// and returns that record's ID.
func (k *keeper) Deposit(ctx sdk.Context, from sdk.AccAddress, to types.SolanaAddress, amount sdk.Coin) (types.ID, error) {
	cCtx, write := ctx.CacheContext()

	p, err := k.params.Get(cCtx)
	if err != nil {
		return types.ID{}, err
	}

	id, err := k.nextID(cCtx, from)
	if err != nil {
		return types.ID{}, err
	}

	exists, err := k.transfers.Has(cCtx, id)
	if err != nil {
		return types.ID{}, err
	}

	if exists {
		return types.ID{}, types.ErrTransferExists
	}

	if amount.IsLT(p.MinAmountPerTransfer) {
		return types.ID{}, types.ErrBelowMinTransfer
	}

	if amount.IsGT(p.MaxAmountPerTransfer) {
		return types.ID{}, types.ErrAboveMaxTransfer
	}

	policy, err := k.GetCurrentPolicy(cCtx)
	if err != nil {
		return types.ID{}, err
	}

	if len(policy.Members) == 0 {
		return types.ID{}, types.ErrInvalidReporters
	}

	epoch, err := k.getEpochID(cCtx)
	if err != nil {
		return types.ID{}, err
	}

	if epoch == 0 {
		return types.ID{}, types.ErrInvalidClock.Wrap("quota epoch has not started")
	}

	u, err := k.GetAccountUsage(cCtx, from)
	if err != nil {
		return types.ID{}, err
	}

	u, err = u.At(epoch)
	if err != nil {
		return types.ID{}, err
	}

	send, err := k.sendUsage.Get(cCtx)
	if err != nil {
		return types.ID{}, err
	}

	send, err = send.At(epoch)
	if err != nil {
		return types.ID{}, err
	}

	nextAccount := u.EpochSent.Add(amount)
	nextSend := send.Amount.Add(amount)

	if p.MaxEpochAmountPerAccount.IsPositive() && nextAccount.IsGT(p.MaxEpochAmountPerAccount) {
		return types.ID{}, types.ErrAccountDailyAmountExceeded
	}

	if nextSend.IsGT(p.MaxTransferAmountPerEpoch) {
		return types.ID{}, types.ErrSendCapReached
	}

	if err = k.bank.SendCoinsFromAccountToModule(cCtx, from, types.ModuleName, sdk.Coins{amount}); err != nil {
		return types.ID{}, err
	}

	rData := types.RecordData{
		Dest:   to.String(),
		Amount: amount,
	}

	if err = k.transfers.Set(cCtx, id, rData); err != nil {
		return types.ID{}, err
	}

	u.TotalSent = u.TotalSent.Add(amount)
	u.EpochSent = nextAccount

	send.Amount = nextSend

	if err = k.usage.Set(cCtx, from, u); err != nil {
		return types.ID{}, err
	}

	if err = k.sendUsage.Set(cCtx, send); err != nil {
		return types.ID{}, err
	}

	if err = k.storeTransfer(cCtx, nil, types.TransferState{
		ID:     id,
		Data:   rData,
		Status: types.StatusTransferPending,
		Settlement: types.SettlementData{
			Round:  1,
			Policy: policy,
		},
	}); err != nil {
		return types.ID{}, err
	}

	if err = cCtx.EventManager().EmitTypedEvent(&types.EventTransferCreated{
		ID:        id,
		Sender:    from.String(),
		Recipient: to.String(),
		Amount:    amount,
		Height:    id.Height,
	}); err != nil {
		return types.ID{}, err
	}

	write()

	return id, nil
}
