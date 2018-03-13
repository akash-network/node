package marketplace

import (
	"context"
	"sync"

	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/tendermint/tendermint/rpc/client"
	tmtmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tmlibs/log"
	"github.com/tendermint/tmlibs/pubsub"
)

type Monitor interface {
	AddHandler(name string, h Handler, q pubsub.Query)
	Start() error
	Stop() error
	Wait() <-chan struct{}
}

type monitor struct {
	client EventProvider

	ctx context.Context
	log log.Logger

	donech chan struct{}
	stopch chan struct{}

	wg sync.WaitGroup
}

// limited client.HTTP to make testing easier.
type EventProvider interface {
	client.EventsClient
	Start() error
	Stop() error
	Wait()
}

func NewMonitor(ctx context.Context, log log.Logger, client EventProvider) Monitor {

	m := &monitor{
		client: client,
		ctx:    ctx,
		log:    log,
		donech: make(chan struct{}),
		stopch: make(chan struct{}),
		wg:     sync.WaitGroup{},
	}

	go m.doWait()

	m.wg.Add(1)
	go m.watchContext()

	return m
}

func (m *monitor) doWait() {
	m.client.Wait()
	close(m.stopch)
	m.wg.Wait()
	close(m.donech)
}

func (m *monitor) watchContext() {
	defer m.wg.Done()

	select {
	case <-m.ctx.Done():
		m.Stop()
	case <-m.stopch:
	}

}

func (m *monitor) Start() error {
	return m.client.Start()
}

func (m *monitor) Stop() error {
	return m.client.Stop()
}

func (m *monitor) Wait() <-chan struct{} {
	return m.donech
}

func (m *monitor) AddHandler(name string, h Handler, q pubsub.Query) {
	ch := m.newListener(h)
	m.client.Subscribe(m.ctx, name, q, ch)
}

func (m *monitor) newListener(h Handler) chan<- interface{} {
	ch := make(chan interface{})

	m.wg.Add(1)
	go m.runListener(ch, h)

	return ch
}

func (m *monitor) runListener(ch <-chan interface{}, h Handler) {
	defer m.wg.Done()

loop:
	for {
		select {
		case <-m.stopch:
			return
		case ev := <-ch:
			ed, ok := ev.(tmtmtypes.TMEventData)
			if !ok {
				continue loop
			}

			evt, ok := ed.Unwrap().(tmtmtypes.EventDataTx)
			if !ok {
				continue loop
			}

			tx, err := txutil.ProcessTx(evt.Tx)
			if err != nil {
				continue loop
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
			}
		}
	}
}
