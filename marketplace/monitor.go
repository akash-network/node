package marketplace

import (
	"context"

	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tmlibs/log"
	"github.com/tendermint/tmlibs/pubsub"
)

type Monitor interface {
	Stop() error
	Wait() <-chan struct{}
}

type monitor struct {
	name    string
	handler Handler
	query   pubsub.Query

	bus tmtmtypes.EventBusSubscriber

	ctx context.Context
	log log.Logger

	donech chan struct{}
}

func NewMonitor(ctx context.Context, log log.Logger, bus tmtmtypes.EventBusSubscriber, name string, handler Handler, query pubsub.Query) (Monitor, error) {

	m := &monitor{
		name:    name,
		handler: handler,
		query:   query,
		ctx:     ctx,
		log:     log,
		bus:     bus,
		donech:  make(chan struct{}),
	}

	ch := make(chan interface{})
	go m.runListener(ch, m.handler)

	if err := m.bus.Subscribe(m.ctx, m.name, m.query, ch); err != nil {
		close(ch)
		<-m.donech
		return nil, err
	}

	return m, nil
}

func (m *monitor) Stop() error {
	return m.bus.Unsubscribe(m.ctx, m.name, m.query)
}

func (m *monitor) Wait() <-chan struct{} {
	return m.donech
}

func (m *monitor) runListener(ch <-chan interface{}, h Handler) {
	defer close(m.donech)

	for ev := range ch {

		ed, ok := ev.(tmtmtypes.TMEventData)
		if !ok {
			continue
		}

		evt, ok := ed.(tmtmtypes.EventDataTx)
		if !ok {
			continue
		}

		tx, err := txutil.ProcessTx(evt.Tx)
		if err != nil {
			m.log.Error("ProcessTx", "error", err)
			continue
		}

		switch tx := tx.Payload.GetPayload().(type) {
		case *types.TxPayload_TxSend:
			h.OnTxSend(tx.TxSend)
		case *types.TxPayload_TxCreateProvider:
			h.OnTxCreateProvider(tx.TxCreateProvider)
		case *types.TxPayload_TxCreateDeployment:
			h.OnTxCreateDeployment(tx.TxCreateDeployment)
		case *types.TxPayload_TxCreateOrder:
			h.OnTxCreateOrder(tx.TxCreateOrder)
		case *types.TxPayload_TxCreateFulfillment:
			h.OnTxCreateFulfillment(tx.TxCreateFulfillment)
		case *types.TxPayload_TxCreateLease:
			h.OnTxCreateLease(tx.TxCreateLease)
		case *types.TxPayload_TxCloseDeployment:
			h.OnTxCloseDeployment(tx.TxCloseDeployment)
		case *types.TxPayload_TxCloseFulfillment:
			h.OnTxCloseFulfillment(tx.TxCloseFulfillment)
		case *types.TxPayload_TxCloseLease:
			h.OnTxCloseLease(tx.TxCloseLease)
		}
	}
}
