package marketplace

import (
	"github.com/ovrclk/photon/types"
)

type Handler interface {
	OnTxSend(*types.TxSend)
	OnTxCreateDatacenter(*types.TxCreateDatacenter)
	OnTxCreateDeployment(*types.TxCreateDeployment)
	OnTxCreateDeploymentOrder(*types.TxCreateDeploymentOrder)
	OnTxCreateFulfillmentOrder(*types.TxCreateFulfillmentOrder)
	OnTxCreateLease(*types.TxCreateLease)
}

type handler struct {
	onTxSend                   func(*types.TxSend)
	onTxCreateDatacenter       func(*types.TxCreateDatacenter)
	onTxCreateDeployment       func(*types.TxCreateDeployment)
	onTxCreateDeploymentOrder  func(*types.TxCreateDeploymentOrder)
	onTxCreateFulfillmentOrder func(*types.TxCreateFulfillmentOrder)
	onTxCreateLease            func(*types.TxCreateLease)
}

func (h handler) OnTxSend(tx *types.TxSend) {
	if h.onTxSend != nil {
		h.onTxSend(tx)
	}
}

func (h handler) OnTxCreateDatacenter(tx *types.TxCreateDatacenter) {
	if h.onTxCreateDatacenter != nil {
		h.onTxCreateDatacenter(tx)
	}
}

func (h handler) OnTxCreateDeployment(tx *types.TxCreateDeployment) {
	if h.onTxCreateDeployment != nil {
		h.onTxCreateDeployment(tx)
	}
}

func (h handler) OnTxCreateDeploymentOrder(tx *types.TxCreateDeploymentOrder) {
	if h.onTxCreateDeploymentOrder != nil {
		h.onTxCreateDeploymentOrder(tx)
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
	OnTxCreateDatacenter(func(*types.TxCreateDatacenter)) Builder
	OnTxCreateDeployment(func(*types.TxCreateDeployment)) Builder
	OnTxCreateDeploymentOrder(func(*types.TxCreateDeploymentOrder)) Builder
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

func (b *builder) OnTxCreateDatacenter(fn func(*types.TxCreateDatacenter)) Builder {
	b.onTxCreateDatacenter = fn
	return b
}

func (b *builder) OnTxCreateDeployment(fn func(*types.TxCreateDeployment)) Builder {
	b.onTxCreateDeployment = fn
	return b
}

func (b *builder) OnTxCreateDeploymentOrder(fn func(*types.TxCreateDeploymentOrder)) Builder {
	b.onTxCreateDeploymentOrder = fn
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
