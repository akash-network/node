package market

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/ovrclk/photon/state"
	tmtypes "github.com/tendermint/abci/types"
	ctypes "github.com/tendermint/tendermint/consensus/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tmlibs/log"
)

const (
	subscriber = "photon-market"
)

type Driver interface {
	OnBeginBlock(req tmtypes.RequestBeginBlock) error
	OnCommit(state state.State) error
}

type driver struct {
	log   log.Logger
	actor Actor

	facilitator Facilitator

	block *tmtypes.RequestBeginBlock
	rs    *ctypes.RoundState

	mtx *sync.Mutex
}

func NewDriver(log log.Logger, actor Actor, bus *tmtmtypes.EventBus) (Driver, error) {

	f := &driver{
		log:         log,
		actor:       actor,
		facilitator: DefaultFacilitator(log, actor),
		mtx:         new(sync.Mutex),
	}

	ch := make(chan interface{})

	if err := bus.Subscribe(context.Background(), "photon-market", tmtmtypes.EventQueryCompleteProposal, ch); err != nil {
		return nil, err
	}

	go func(ch chan interface{}) {
		for evt := range ch {
			f.onEvent(evt)
		}
	}(ch)

	return f, nil
}

func (d *driver) OnBeginBlock(req tmtypes.RequestBeginBlock) error {
	d.block = &req
	return nil
}

func (d *driver) OnCommit(state state.State) error {
	if !d.checkCommit(state) {
		return nil
	}

	// XXX: hack to get around deadlock when sending txs to the rpc server.
	go func() {
		d.facilitator.Run(state)
	}()

	return nil
}

func (d *driver) checkCommit(state state.State) bool {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	if d.block == nil || d.rs == nil {
		return false
	}

	if d.rs.Height != state.Version() {
		return false
	}

	if bytes.Compare(d.actor.Address(), d.rs.Validators.GetProposer().PubKey.Address()) != 0 {
		return false
	}

	if bytes.Compare(d.block.Hash, d.rs.ProposalBlock.Header.Hash()) != 0 {
		return false
	}

	return true
}

func (d *driver) onProposalComplete(rs *ctypes.RoundState) {
	d.log.Info("proposal complete",
		"height", rs.Height,
		"proposer", hex.EncodeToString(rs.Validators.GetProposer().PubKey.Address()),
		"block-hash", rs.ProposalBlock.Header.Hash())
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.rs = rs
}

func (d *driver) onEvent(evt interface{}) {

	tmed, ok := evt.(tmtmtypes.TMEventData)
	if !ok {
		d.log.Error("bad event type", "type", fmt.Sprintf("%T", evt))
		return
	}

	edrs, ok := tmed.Unwrap().(tmtmtypes.EventDataRoundState)
	if !ok {
		d.log.Error("bad event data type", "type", fmt.Sprintf("%T", tmed))
		return
	}

	if edrs.RoundState == nil {
		d.log.Error("nil round state")
		return
	}

	rs, ok := edrs.RoundState.(*ctypes.RoundState)
	if !ok {
		d.log.Error("bad round state type", "type", fmt.Sprintf("%T", edrs.RoundState))
		return
	}

	d.onProposalComplete(rs)
}
