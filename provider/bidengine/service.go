package bidengine

import (
	"context"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
)

type Service interface {
	Close() error
	Done() <-chan struct{}
}

// Service handles bidding on orders.
func NewService(ctx context.Context, session session.Session, cluster cluster.Cluster, bus event.Bus) (Service, error) {

	sub, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}

	s := &service{
		session: session.ForModule("bidengine-service"),
		cluster: cluster,
		bus:     bus,
		sub:     sub,
		orders:  make(map[*order]bool),
		drainch: make(chan *order),
		lc:      lifecycle.New(),
	}

	go s.lc.WatchContext(ctx)
	go s.run()

	return s, nil
}

type service struct {
	session session.Session
	cluster cluster.Cluster

	bus event.Bus
	sub event.Subscriber

	orders  map[*order]bool
	drainch chan *order

	lc lifecycle.Lifecycle
}

func (s *service) Close() error {
	s.lc.Shutdown(nil)
	return s.lc.Error()
}

func (s *service) Done() <-chan struct{} {
	return s.lc.Done()
}

func (s *service) run() {
	defer s.lc.ShutdownCompleted()
	defer s.sub.Close()

loop:
	for {
		select {
		case <-s.lc.ShutdownRequest():
			s.lc.ShutdownInitiated(nil)
			break loop

		case ev := <-s.sub.Events():
			switch ev := ev.(type) {
			case *event.TxCreateOrder:
				// new order
				opath := keys.OrderID(ev.OrderID).Path()

				s.session.Log().Info("order detected", "order", opath)

				// create an order object for managing the bid process and order lifecycle
				order, err := newOrder(s, ev)

				if err != nil {
					// todo: handle error
					s.session.Log().Error("handling order", "order", opath, "err", err)
					break
				}

				s.orders[order] = true

			}
		case order := <-s.drainch:
			// child done
			delete(s.orders, order)
		}
	}

	// drain: wait for all order monitors to complete.
	for len(s.orders) > 0 {
		delete(s.orders, <-s.drainch)
	}

}
