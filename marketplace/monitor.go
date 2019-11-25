package marketplace

import (
	"context"

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

	txtch, err := m.bus.Subscribe(m.ctx, m.name+"-tx", TxQuery().String(), 100)
	if err != nil {
		return nil, err
	}

	blkch, err := m.bus.Subscribe(m.ctx, m.name+"-blk", BlkQuery().String(), 100)
	if err != nil {
		return nil, err
	}

	ch := make(chan ctypes.ResultEvent, 100)

	go func() {
		for {
			select {
			case val := <-txtch:
				select {
				case ch <- val:
				case <-m.donech:
				}
			case val := <-blkch:
				select {
				case ch <- val:
				case <-m.donech:
				}
			case <-m.donech:
				return
			}
		}
	}()

	go m.runListener(handler, ch)

	return m, nil
}

func (m *monitor) Stop() error {
	close(m.donech)
	m.bus.UnsubscribeAll(m.ctx, m.name+"-tx")
	return m.bus.UnsubscribeAll(m.ctx, m.name+"-blk")
}

func (m *monitor) Wait() <-chan struct{} {
	return m.donech
}

func (m *monitor) runListener(h Handler, ch <-chan ctypes.ResultEvent) {
	for {
		select {
		case ed := <-ch:

			// fmt.Println("ed", ed.Events)

			switch evt := ed.Data.(type) {
			case tmtmtypes.EventDataTx:

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
			// case tmtmtypes.EventDataNewBlock:

			case tmtmtypes.EventDataNewBlockHeader:

				// fmt.Println("event data", evt)
				// fmt.Println("new block header", evt.ResultEndBlock.Events)

				for _, ev := range evt.ResultEndBlock.Events {
					// fmt.Println("handling event", ev.GetType(), ev.String())
					switch ev.GetType() {
					case "market.lease-create":
						tx, err := unmarshalLeaseCreate(ev)
						// fmt.Println("lease unmarshal:", tx, err)
						if err == nil && tx != nil {
							h.OnTxCreateLease(tx)
						}
					case "market.order-create":
						tx, err := unmarshalOrderCreate(ev)
						// fmt.Println("order unmarshal", tx, err)
						if err == nil && tx != nil {
							// fmt.Println("sending create order")
							h.OnTxCreateOrder(tx)
						}
					case "market.lease-close":
						tx, err := unmarshalLeaseClose(ev)
						if err == nil && tx != nil {
							h.OnTxCloseLease(tx)
						}
					}
				}
				// ed map[market.order-create.order-id:[2a4b22c83e2c95aa6bd30b93636709f5105a069fcdb12cb1a207aa8088e32a6a/1/2] tm.event:[NewBlockHeader]]

				// new block header [{market.order-create [{[111 114 100 101 114 45 105 100] [50 97 52 98 50 50 99 56 51 101 50 99 57 53 97 97 54 98 100 51 48 98 57 51 54 51 54 55 48 57 102 53 49 48 53 97 48 54 57 102 99 100 98 49 50 99 98 49 97 50 48 55 97 97 56 48 56 56 101 51 50 97 54 97 47 49 47 50] {} [] 0}] {} [] 0}]

				// ed map[deployment-create.app:[deployment] tm.event:[Tx] tx.hash:[7A47B7986C147B047FFCE7A210DBF3BD8112BFCD978D4999FE1C5C1F7C42C2C3] tx.height:[1142]]
				// DEPLOYMENT CREATED	2a4b22c83e2c95aa6bd30b93636709f5105a069fcdb12cb1a207aa8088e32a6a created by 5dc10c91bc3844dfa8d0f4de4049c9c78187a51b

			}

		case <-m.donech:
			return
		}
	}
}
