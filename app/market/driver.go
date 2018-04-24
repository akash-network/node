package market

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/util"
	tmtypes "github.com/tendermint/abci/types"
	ctypes "github.com/tendermint/tendermint/consensus/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tmlibs/log"
)

const (
	subscriber = "akash-market"
)

type Driver interface {
	OnBeginBlock(req tmtypes.RequestBeginBlock) error
	OnCommit(state state.State) error
	Stop()
}

type driver struct {
	log   log.Logger
	actor Actor

	evch        chan interface{}
	facilitator Facilitator

	block *tmtypes.RequestBeginBlock
	rs    *ctypes.RoundState

	bus *tmtmtypes.EventBus

	ctx    context.Context
	cancel context.CancelFunc
	mtx    *sync.Mutex
	wg     sync.WaitGroup
}

func NewDriver(ctx context.Context, log log.Logger, actor Actor, bus *tmtmtypes.EventBus) (Driver, error) {
	return NewDriverWithFacilitator(ctx, log, actor, bus, DefaultFacilitator(ctx, log, actor))
}

func NewDriverWithFacilitator(ctx context.Context, log log.Logger, actor Actor, bus *tmtmtypes.EventBus, facilitator Facilitator) (Driver, error) {
	ctx, cancel := context.WithCancel(ctx)

	d := &driver{
		log:         log,
		actor:       actor,
		evch:        make(chan interface{}, 1),
		facilitator: facilitator,
		bus:         bus,
		ctx:         ctx,
		cancel:      cancel,
		mtx:         new(sync.Mutex),
		wg:          sync.WaitGroup{},
	}

	if err := d.subscribe(); err != nil {
		return nil, err
	}

	d.wg.Add(2)
	go d.watchContext()
	go d.run()

	return d, nil
}

func (d *driver) Stop() {
	d.cancel()
	d.wg.Wait()
}

func (d *driver) OnBeginBlock(req tmtypes.RequestBeginBlock) error {
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

	if d.block == nil || d.rs == nil {
		d.log.Debug("no block or rs")
		return false
	}

	if d.rs.Height != state.Version() {
		d.log.Error("bad height", "rs", d.rs.Height, "state", state.Version())
		return false
	}

	if bytes.Compare(d.actor.Address(), d.rs.Validators.GetProposer().PubKey.Address()) != 0 {
		d.log.Debug("wrong address", "actor", util.X(d.actor.Address()), "proposer", util.X(d.rs.Validators.GetProposer().PubKey.Address()))
		return false
	}

	if bytes.Compare(d.block.Hash, d.rs.ProposalBlock.Header.Hash()) != 0 {
		d.log.Info("bad block hash", "block", util.X(d.block.Hash), "proposal", d.rs.ProposalBlock.Header.Hash())
		return false
	}

	return true
}

func (d *driver) onProposalComplete(rs *ctypes.RoundState) {
	d.log.Info("proposal complete",
		"height", rs.Height,
		"proposer", util.X(rs.Validators.GetProposer().PubKey.Address()),
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

	edrs, ok := tmed.(tmtmtypes.EventDataRoundState)
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

func (d *driver) watchContext() {
	defer d.wg.Done()
	<-d.ctx.Done()
	if err := d.unsubscribe(); err != nil {
		d.log.Error("unsubscribing", "error", err)
	}
}

func (d *driver) subscribe() error {
	return d.bus.Subscribe(d.ctx, subscriber, tmtmtypes.EventQueryCompleteProposal, d.evch)
}

func (d *driver) unsubscribe() error {
	// TODO: better interface with tm event bus
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*5))
	defer cancel()
	return d.bus.Unsubscribe(ctx, subscriber, tmtmtypes.EventQueryCompleteProposal)
}

func (d *driver) run() {
	defer d.wg.Done()
	for ev := range d.evch {
		d.onEvent(ev)
	}
}
