package market

import (
	"bytes"
	"context"
	"sync"

	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/util"
	abci_types "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
)

type Driver interface {
	OnBeginBlock(req abci_types.RequestBeginBlock) error
	OnCommit(state state.State) error
}

type driver struct {
	log         log.Logger
	actor       Actor
	facilitator Facilitator

	block *abci_types.RequestBeginBlock

	mtx *sync.Mutex
}

func NewDriver(ctx context.Context, log log.Logger, actor Actor) (Driver, error) {
	return NewDriverWithFacilitator(log, actor, DefaultFacilitator(ctx, log, actor))
}

func NewDriverWithFacilitator(log log.Logger, actor Actor, facilitator Facilitator) (Driver, error) {

	d := &driver{
		log:         log,
		actor:       actor,
		facilitator: facilitator,
		mtx:         new(sync.Mutex),
	}

	return d, nil
}

func (d *driver) OnBeginBlock(req abci_types.RequestBeginBlock) error {
	d.block = &req
	return nil
}

func (d *driver) OnCommit(state state.State) error {
	if !d.checkCommit(state) {
		return nil
	}
	d.log.Debug("running facilitator...")
	return d.facilitator.Run(state)
}

func (d *driver) checkCommit(state state.State) bool {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	defer func() {
		d.block = nil
	}()

	if d.block == nil {
		d.log.Debug("no block")
		return false
	}

	if !bytes.Equal(d.actor.Address(), d.block.Header.GetProposerAddress()) {
		d.log.Debug("not our proposal.  skipping.",
			"actor", util.X(d.actor.Address()), "proposer",
			util.X(d.block.Header.GetProposerAddress()))
		return false
	}

	return true
}
