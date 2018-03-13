package marketplace

import (
	"github.com/ovrclk/photon/types"
)

type Handler interface {
	OnTxSend(*types.TxSend)
	OnTxCreateProvider(*types.TxCreateProvider)
	OnTxCreateDeployment(*types.TxCreateDeployment)
	OnTxCreateOrder(*types.TxCreateOrder)
	OnTxCreateFulfillmentOrder(*types.TxCreateFulfillmentOrder)
	OnTxCreateLease(*types.TxCreateLease)
}

type handler struct {
	onTxSend                   func(*types.TxSend)
	onTxCreateProvider         func(*types.TxCreateProvider)
	onTxCreateDeployment       func(*types.TxCreateDeployment)
	onTxCreateOrder            func(*types.TxCreateOrder)
	onTxCreateFulfillmentOrder func(*types.TxCreateFulfillmentOrder)
	onTxCreateLease            func(*types.TxCreateLease)
}

func (h handler) OnTxSend(tx *types.TxSend) {
	if h.onTxSend != nil {
		h.onTxSend(tx)
	}
}

func (h handler) OnTxCreateProvider(tx *types.TxCreateProvider) {
	if h.onTxCreateProvider != nil {
		h.onTxCreateProvider(tx)
	}
}

func (h handler) OnTxCreateDeployment(tx *types.TxCreateDeployment) {
	if h.onTxCreateDeployment != nil {
		h.onTxCreateDeployment(tx)
	}
}

func (h handler) OnTxCreateOrder(tx *types.TxCreateOrder) {
	if h.onTxCreateOrder != nil {
		h.onTxCreateOrder(tx)
	}
}

func (h handler) OnTxCreateFulfillmentOrder(tx *types.TxCreateFulfillmentOrder) {
	if h.onTxCreateFulfillmentOrder != nil {
		h.onTxCreateFulfillmentOrder(tx)
	}
}

func (h handler) OnTxCreateLease(tx *types.TxCreateLease) {
	if h.onTxCreateLease != nil {
		h.onTxCreateLease(tx)
	}
}

type Builder interface {
	OnTxSend(func(*types.TxSend)) Builder
	OnTxCreateProvider(func(*types.TxCreateProvider)) Builder
	OnTxCreateDeployment(func(*types.TxCreateDeployment)) Builder
	OnTxCreateOrder(func(*types.TxCreateOrder)) Builder
	OnTxCreateFulfillmentOrder(func(*types.TxCreateFulfillmentOrder)) Builder
	OnTxCreateLease(func(*types.TxCreateLease)) Builder
	Create() Handler
}

type builder handler

func NewBuilder() Builder {
	return &builder{}
}

func (b *builder) OnTxSend(fn func(*types.TxSend)) Builder {
	b.onTxSend = fn
	return b
}

func (b *builder) OnTxCreateProvider(fn func(*types.TxCreateProvider)) Builder {
	b.onTxCreateProvider = fn
	return b
}

func (b *builder) OnTxCreateDeployment(fn func(*types.TxCreateDeployment)) Builder {
	b.onTxCreateDeployment = fn
	return b
}

func (b *builder) OnTxCreateOrder(fn func(*types.TxCreateOrder)) Builder {
	b.onTxCreateOrder = fn
	return b
}

func (b *builder) OnTxCreateFulfillmentOrder(fn func(*types.TxCreateFulfillmentOrder)) Builder {
	b.onTxCreateFulfillmentOrder = fn
	return b
}

func (b *builder) OnTxCreateLease(fn func(*types.TxCreateLease)) Builder {
	b.onTxCreateLease = fn
	return b
}

func (b *builder) Create() Handler {
	return (handler)(*b)
}
