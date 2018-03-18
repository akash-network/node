package market

import (
	"context"
	"errors"

	"github.com/ovrclk/akash/state"
)

func NewWorker(ctx context.Context, delegate Facilitator) Facilitator {
	w := &worker{
		delegate: delegate,
		runch:    make(chan state.State),
		ctx:      ctx,
	}
	go w.run()
	return w
}

type worker struct {
	delegate Facilitator
	runch    chan state.State
	ctx      context.Context
}

func (w *worker) run() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case state := <-w.runch:
			w.delegate.Run(state)
		}
	}
}

func (w *worker) Run(state state.State) error {
	select {
	case w.runch <- state:
		return nil
	default:
		return errors.New("market worker: overflow")
	}
}
