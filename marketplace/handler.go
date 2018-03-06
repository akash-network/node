package marketplace

import (
	"github.com/ovrclk/photon/types"
)

type Handler interface {
	OnTxSend(*types.TxSend)
	OnTxCreateDeployment(*types.TxCreateDeployment)
	OnTxCreateDatacenter(*types.TxCreateDatacenter)
}

type handler struct {
	onTxSend             func(*types.TxSend)
	onTxCreateDeployment func(*types.TxCreateDeployment)
	onTxCreateDatacenter func(*types.TxCreateDatacenter)
}

func (h handler) OnTxSend(tx *types.TxSend) {
	if h.onTxSend != nil {
		h.onTxSend(tx)
	}
}

func (h handler) OnTxCreateDeployment(tx *types.TxCreateDeployment) {
	if h.onTxCreateDeployment != nil {
		h.onTxCreateDeployment(tx)
	}
}

func (h handler) OnTxCreateDatacenter(tx *types.TxCreateDatacenter) {
	if h.onTxCreateDatacenter != nil {
		h.onTxCreateDatacenter(tx)
	}
}

type Builder interface {
	OnTxSend(func(*types.TxSend)) Builder
	OnTxCreateDeployment(func(*types.TxCreateDeployment)) Builder
	OnTxCreateDatacenter(func(*types.TxCreateDatacenter)) Builder
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

func (b *builder) OnTxCreateDeployment(fn func(*types.TxCreateDeployment)) Builder {
	b.onTxCreateDeployment = fn
	return b
}

func (b *builder) OnTxCreateDatacenter(fn func(*types.TxCreateDatacenter)) Builder {
	b.onTxCreateDatacenter = fn
	return b
}

func (b *builder) Create() Handler {
	return (handler)(*b)
}
