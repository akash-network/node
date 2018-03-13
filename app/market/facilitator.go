package market

import (
	"github.com/ovrclk/akash/state"
	"github.com/tendermint/tmlibs/log"
)

type Facilitator interface {
	Run(state.State) error
}

type facilitator struct {
	actor  Actor
	engine Engine
	client Client
	log    log.Logger
}

func DefaultFacilitator(log log.Logger, actor Actor) Facilitator {
	return NewFacilitator(log, actor, NewEngine(log), newLocalClient())
}

func NewFacilitator(log log.Logger, actor Actor, engine Engine, client Client) Facilitator {
	return &facilitator{
		actor:  actor,
		engine: engine,
		client: client,
		log:    log,
	}
}

func (f *facilitator) Run(state state.State) error {

	txs, err := f.engine.Run(state)
	if err != nil {
		return err
	}

	f.log.Info("engine ran", "count", len(txs))

	nonce, err := f.currentNonce(state)
	if err != nil {
		return err
	}

	sender := NewSender(f.log, f.client, f.actor, nonce)

	for _, tx := range txs {
		if _, err := sender.Send(tx); err != nil {
			f.log.Error("Error sending transaction", err)
			return err
		}
	}

	return nil
}

func (f *facilitator) currentNonce(state state.State) (uint64, error) {
	account, err := state.Account().Get(f.actor.Address())
	if err != nil {
		f.log.Error("Facilitator does not have an account.", err)
		return 0, err
	}
	if account == nil {
		return 1, nil
	}
	return account.Nonce, nil
}
