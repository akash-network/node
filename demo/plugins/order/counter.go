package counter

import (
	"fmt"

	"github.com/tendermint/go-wire"

	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/errors"
	"github.com/cosmos/cosmos-sdk/modules/auth"
	"github.com/cosmos/cosmos-sdk/modules/base"
	"github.com/cosmos/cosmos-sdk/modules/coin"
	"github.com/cosmos/cosmos-sdk/modules/fee"
	"github.com/cosmos/cosmos-sdk/modules/ibc"
	"github.com/cosmos/cosmos-sdk/modules/nonce"
	"github.com/cosmos/cosmos-sdk/modules/roles"
	"github.com/cosmos/cosmos-sdk/stack"
	"github.com/cosmos/cosmos-sdk/state"
)

/*

	TODO: re-write this to support order transactions

	this will be sent by a user to declare they want some service specifid in the transaction data

*/

// Tx
//--------------------------------------------------------------------------------

// register the tx type with it's validation logic
// make sure to use the name of the handler as the prefix in the tx type,
// so it gets routed properly
const (
	NameCounter = "cntr"
	ByteTx      = 0x2F //TODO What does this byte represent should use typebytes probably
	TypeTx      = NameCounter + "/count"
)

func init() {
	sdk.TxMapper.RegisterImplementation(Tx{}, TypeTx, ByteTx)
}

// Tx - struct for all counter transactions
type Tx struct {
	Valid bool       `json:"valid"`
	Fee   coin.Coins `json:"fee"`
}

// NewTx - return a new counter transaction struct wrapped as a basecoin transaction
func NewTx(valid bool, fee coin.Coins) sdk.Tx {
	return Tx{
		Valid: valid,
		Fee:   fee,
	}.Wrap()
}

// Wrap - Wrap a Tx as a Basecoin Tx, used to satisfy the XXX interface
func (c Tx) Wrap() sdk.Tx {
	return sdk.Tx{TxInner: c}
}

// ValidateBasic just makes sure the Fee is a valid, non-negative value
func (c Tx) ValidateBasic() error {
	if !c.Fee.IsValid() {
		return coin.ErrInvalidCoins()
	}
	if !c.Fee.IsNonnegative() {
		return coin.ErrInvalidCoins()
	}
	return nil
}

// Custom errors
//--------------------------------------------------------------------------------

var (
	errInvalidCounter = fmt.Errorf("Counter Tx marked invalid")
)

// ErrInvalidCounter - custom error class
func ErrInvalidCounter() error {
	return errors.WithCode(errInvalidCounter, errors.CodeTypeBaseInvalidInput)
}

// IsInvalidCounterErr - custom error class check
func IsInvalidCounterErr(err error) bool {
	return errors.IsSameError(errInvalidCounter, err)
}

// ErrDecoding - This is just a helper function to return a generic "internal error"
func ErrDecoding() error {
	return errors.ErrInternal("Error decoding state")
}

// Counter Handler
//--------------------------------------------------------------------------------

/* snippet from basecoin example
		diff is roles (multisig), ibc (inter blockchain), eyes (kv store)

		Dispatch(
			coin.NewHandler(),
			stack.WrapHandler(roles.NewHandler()),
			stack.WrapHandler(ibc.NewHandler()),
			// and just for run, add eyes as well
			stack.WrapHandler(eyes.NewHandler()),
		)
}
*/

// NewHandler returns a new counter transaction processing handler
func NewHandler(feeDenom string) sdk.Handler {
	return stack.New(
		base.Logger{},
		stack.Recovery{},
		auth.Signatures{},
		base.Chain{},
		stack.Checkpoint{OnCheck: true},
		nonce.ReplayCheck{},
	).
		IBC(ibc.NewMiddleware()).
		Apps(
			roles.NewMiddleware(),
			fee.NewSimpleFeeMiddleware(coin.Coin{feeDenom, 0}, fee.Bank),
			stack.Checkpoint{OnDeliver: true},
		).
		Dispatch(
			coin.NewHandler(),
			Handler{},
		)
}

// Handler the counter transaction processing handler
type Handler struct {
	stack.PassInitState
	stack.PassInitValidate
}

var _ stack.Dispatchable = Handler{}

// Name - return counter namespace
func (Handler) Name() string {
	return NameCounter
}

// AssertDispatcher - placeholder to satisfy XXX
func (Handler) AssertDispatcher() {}

// CheckTx checks if the tx is properly structured
func (h Handler) CheckTx(ctx sdk.Context, store state.SimpleDB, tx sdk.Tx, _ sdk.Checker) (res sdk.CheckResult, err error) {
	_, err = checkTx(ctx, tx)
	return
}

// DeliverTx executes the tx if valid
func (h Handler) DeliverTx(ctx sdk.Context, store state.SimpleDB, tx sdk.Tx, dispatch sdk.Deliver) (res sdk.DeliverResult, err error) {
	ctr, err := checkTx(ctx, tx)
	if err != nil {
		return res, err
	}
	// note that we don't assert this on CheckTx (ValidateBasic),
	// as we allow them to be writen to the chain
	if !ctr.Valid {
		return res, ErrInvalidCounter()
	}

	// handle coin movement.... like, actually decrement the other account
	if !ctr.Fee.IsZero() {
		// take the coins and put them in out account!
		senders := ctx.GetPermissions("", auth.NameSigs)
		if len(senders) == 0 {
			return res, errors.ErrMissingSignature()
		}
		in := []coin.TxInput{{Address: senders[0], Coins: ctr.Fee}}
		out := []coin.TxOutput{{Address: StoreActor(), Coins: ctr.Fee}}
		send := coin.NewSendTx(in, out)
		// if the deduction fails (too high), abort the command
		_, err = dispatch.DeliverTx(ctx, store, send)
		if err != nil {
			return res, err
		}
	}

	// update the counter
	state, err := LoadState(store)
	if err != nil {
		return res, err
	}
	state.Counter++
	state.TotalFees = state.TotalFees.Plus(ctr.Fee)
	err = SaveState(store, state)

	return res, err
}

func checkTx(ctx sdk.Context, tx sdk.Tx) (ctr Tx, err error) {
	ctr, ok := tx.Unwrap().(Tx)
	if !ok {
		return ctr, errors.ErrInvalidFormat(TypeTx, tx)
	}
	err = ctr.ValidateBasic()
	if err != nil {
		return ctr, err
	}
	return ctr, nil
}

// CounterStore
//--------------------------------------------------------------------------------

// StoreActor - return the basecoin actor for the account
func StoreActor() sdk.Actor {
	return sdk.Actor{App: NameCounter, Address: []byte{0x04, 0x20}} //XXX what do these bytes represent? - should use typebyte variables
}

// State - state of the counter applicaton
type State struct {
	Counter   int        `json:"counter"`
	TotalFees coin.Coins `json:"total_fees"`
}

// StateKey - store key for the counter state
func StateKey() []byte {
	return []byte("state")
}

// LoadState - retrieve the counter state from the store
func LoadState(store state.SimpleDB) (state State, err error) {
	bytes := store.Get(StateKey())
	if len(bytes) > 0 {
		err = wire.ReadBinaryBytes(bytes, &state)
		if err != nil {
			return state, errors.ErrDecoding()
		}
	}
	return state, nil
}

// SaveState - save the counter state to the provided store
func SaveState(store state.SimpleDB, state State) error {
	bytes := wire.BinaryBytes(state)
	store.Set(StateKey(), bytes)
	return nil
}
