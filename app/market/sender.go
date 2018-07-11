package market

import (
	"github.com/ovrclk/akash/txutil"
	"github.com/tendermint/tendermint/libs/log"
)

type Sender interface {
	Send(payload interface{}) (uint64, error)
}

type sender struct {
	client Client
	actor  Actor
	nonce  uint64
	log    log.Logger
}

func NewSender(log log.Logger, client Client, actor Actor, nonce uint64) Sender {
	return &sender{client: client, actor: actor, nonce: nonce, log: log}
}

func (s *sender) Send(payload interface{}) (uint64, error) {
	nextNonce := s.nonce + 1
	tx, err := s.buildTx(nextNonce, payload)

	if err != nil {
		return s.nonce, err
	}

	s.client.BroadcastTxAsync(tx)

	s.nonce = nextNonce
	return s.nonce, nil
}

func (s *sender) buildTx(nonce uint64, payload interface{}) ([]byte, error) {
	return txutil.BuildTx(txutil.NewPrivateKeySigner(s.actor), nonce, payload)
}
