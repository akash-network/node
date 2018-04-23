package marketplace

import (
	"github.com/ovrclk/akash/types"
)

type Handler interface {
	OnTxSend(*types.TxSend)
	OnTxCreateProvider(*types.TxCreateProvider)
	OnTxCreateDeployment(*types.TxCreateDeployment)
	OnTxCreateOrder(*types.TxCreateOrder)
	OnTxCreateFulfillment(*types.TxCreateFulfillment)
	OnTxCreateLease(*types.TxCreateLease)
	OnTxCloseDeployment(*types.TxCloseDeployment)
	OnTxCloseFulfillment(*types.TxCloseFulfillment)
	OnTxCloseLease(*types.TxCloseLease)
}

type handler struct {
	onTxSend              func(*types.TxSend)
	onTxCreateProvider    func(*types.TxCreateProvider)
	onTxCreateDeployment  func(*types.TxCreateDeployment)
	onTxCreateOrder       func(*types.TxCreateOrder)
	onTxCreateFulfillment func(*types.TxCreateFulfillment)
	onTxCreateLease       func(*types.TxCreateLease)
	onTxCloseDeployment   func(*types.TxCloseDeployment)
	onTxCloseFulfillment  func(*types.TxCloseFulfillment)
	onTxCloseLease        func(*types.TxCloseLease)
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

func (h handler) OnTxCreateFulfillment(tx *types.TxCreateFulfillment) {
	if h.onTxCreateFulfillment != nil {
		h.onTxCreateFulfillment(tx)
	}
}

func (h handler) OnTxCreateLease(tx *types.TxCreateLease) {
	if h.onTxCreateLease != nil {
		h.onTxCreateLease(tx)
	}
}

func (h handler) OnTxCloseDeployment(tx *types.TxCloseDeployment) {
	if h.onTxCloseDeployment != nil {
		h.onTxCloseDeployment(tx)
	}
}

func (h handler) OnTxCloseFulfillment(tx *types.TxCloseFulfillment) {
	if h.onTxCloseFulfillment != nil {
		h.onTxCloseFulfillment(tx)
	}
}

func (h handler) OnTxCloseLease(tx *types.TxCloseLease) {
	if h.onTxCloseLease != nil {
		h.onTxCloseLease(tx)
	}
}

type Builder interface {
	OnTxSend(func(*types.TxSend)) Builder
	OnTxCreateProvider(func(*types.TxCreateProvider)) Builder
	OnTxCreateDeployment(func(*types.TxCreateDeployment)) Builder
	OnTxCreateOrder(func(*types.TxCreateOrder)) Builder
	OnTxCreateFulfillment(func(*types.TxCreateFulfillment)) Builder
	OnTxCreateLease(func(*types.TxCreateLease)) Builder
	OnTxCloseDeployment(func(*types.TxCloseDeployment)) Builder
	OnTxCloseFulfillment(func(*types.TxCloseFulfillment)) Builder
	OnTxCloseLease(func(*types.TxCloseLease)) Builder
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

func (b *builder) OnTxCreateFulfillment(fn func(*types.TxCreateFulfillment)) Builder {
	b.onTxCreateFulfillment = fn
	return b
}

func (b *builder) OnTxCreateLease(fn func(*types.TxCreateLease)) Builder {
	b.onTxCreateLease = fn
	return b
}

func (b *builder) OnTxCloseDeployment(fn func(*types.TxCloseDeployment)) Builder {
	b.onTxCloseDeployment = fn
	return b
}

func (b *builder) OnTxCloseFulfillment(fn func(*types.TxCloseFulfillment)) Builder {
	b.onTxCloseFulfillment = fn
	return b
}

func (b *builder) OnTxCloseLease(fn func(*types.TxCloseLease)) Builder {
	b.onTxCloseLease = fn
	return b
}

func (b *builder) Create() Handler {
	return (handler)(*b)
}
