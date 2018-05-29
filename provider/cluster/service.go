package cluster

import (
	"context"
	"errors"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
)

var ErrNotRunning = errors.New("not running")

type Cluster interface {
	Reserve(types.OrderID, *types.DeploymentGroup) (Reservation, error)
}

type Service interface {
	Cluster
	Close() error
	Done() <-chan struct{}
}

func NewService(log log.Logger, ctx context.Context, bus event.Bus) (Service, error) {

	log = log.With("module", "provider-cluster")

	sub, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}

	s := &service{
		bus:       bus,
		sub:       sub,
		reservech: make(chan reserveRequest),
		log:       log,
		lc:        lifecycle.New(),
	}

	go s.lc.WatchContext(ctx)
	go s.run()

	return s, nil
}

type service struct {
	bus       event.Bus
	sub       event.Subscriber
	reservech chan reserveRequest
	log       log.Logger
	lc        lifecycle.Lifecycle
}

func (s *service) Close() error {
	s.lc.Shutdown(nil)
	return s.lc.Error()
}

func (s *service) Done() <-chan struct{} {
	return s.lc.Done()
}

func (s *service) Reserve(order types.OrderID, group *types.DeploymentGroup) (Reservation, error) {
	ch := make(chan reserveResponse, 1)
	req := reserveRequest{
		order: order,
		group: group,
		ch:    ch,
	}

	select {
	case s.reservech <- req:
		response := <-ch
		return response.value, response.err
	case <-s.lc.ShuttingDown():
		return nil, ErrNotRunning
	}
}

type reserveRequest struct {
	order types.OrderID
	group *types.DeploymentGroup
	ch    chan<- reserveResponse
}

type reserveResponse struct {
	value Reservation
	err   error
}

func (s *service) run() {
	defer s.lc.ShutdownCompleted()
	defer s.sub.Close()

loop:
	for {
		select {
		case err := <-s.lc.ShutdownRequest():
			s.lc.ShutdownInitiated(err)
			break loop

		case req := <-s.reservech:
			// TODO
			req.ch <- reserveResponse{newReservation(req.order, req.group), nil}
		}
	}
}
