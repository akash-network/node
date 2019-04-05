package marketplace

import (
	"context"
	"fmt"

	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/pubsub"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
)

type Monitor interface {
	Stop() error
	Wait() <-chan struct{}
}

type monitor struct {
	name    string
	handler Handler
	query   pubsub.Query

	bus tmclient.EventsClient

	ctx context.Context
	log log.Logger

	donech chan struct{}
}

func NewMonitor(ctx context.Context, log log.Logger, bus tmclient.EventsClient, name string, handler Handler, query pubsub.Query) (Monitor, error) {
	m := &monitor{
		name:    name,
		handler: handler,
		query:   query,
		ctx:     ctx,
		log:     log,
		bus:     bus,
		donech:  make(chan struct{}),
	}

	fmt.Println("monitor --> before subscribe")
	resC, err := m.bus.Subscribe(m.ctx, m.name, m.query.String())
	if err != nil {
		<-m.donech
		return nil, err
	}
	fmt.Println("monitor --> after subscribe")

	go m.runListener(resC, m.handler)

	return m, nil
}

func (m *monitor) Stop() error {
	fmt.Println("monitor --> stop")
	close(m.donech)
	return m.bus.Unsubscribe(m.ctx, m.name, m.query.String())
}

func (m *monitor) Wait() <-chan struct{} {
	fmt.Println("monitor --> wait called")
	return m.donech
}

func (m *monitor) runListener(ch <-chan ctypes.ResultEvent, h Handler) {
	for {
		fmt.Println("monitor --> wait on event")
		select {
		case ed := <-ch:
			evt, ok := ed.Data.(tmtmtypes.EventDataTx)
			if !ok {
				fmt.Println("monitor --> runListener not ok EventDataTx", evt)
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
			case *types.TxPayload_TxUpdateDeployment:
				h.OnTxUpdateDeployment(tx.TxUpdateDeployment)
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
			fmt.Println("monitor --> done processing event")
		case <-m.donech:
			fmt.Println("monitor --> donech closed")
			return
		}
	}
}
