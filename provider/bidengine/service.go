package bidengine

import (
	"context"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/caarlos0/env"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/types"
)

type Service interface {
	Close() error
	Done() <-chan struct{}
}

// Service handles bidding on orders.
func NewService(ctx context.Context, session session.Session, cluster cluster.Cluster, bus event.Bus) (Service, error) {

	config := config{}
	if err := env.Parse(&config); err != nil {
		return nil, err
	}

	sub, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}

	session = session.ForModule("bidengine-service")

	existingOrders, err := queryExistingOrders(ctx, session)
	if err != nil {
		session.Log().Error("finding existing orders", "err", err)
		sub.Close()
		return nil, err
	}
	session.Log().Info("found orders", "count", len(existingOrders))

	s := &service{
		config:  config,
		session: session,
		cluster: cluster,
		bus:     bus,
		sub:     sub,
		orders:  make(map[string]*order),
		drainch: make(chan *order),
		lc:      lifecycle.New(),
	}

	go s.lc.WatchContext(ctx)
	go s.run(existingOrders)

	return s, nil
}

type service struct {
	config  config
	session session.Session
	cluster cluster.Cluster

	bus event.Bus
	sub event.Subscriber

	orders  map[string]*order
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

func (s *service) run(existingOrders []existingOrder) {
	defer s.lc.ShutdownCompleted()
	defer s.sub.Close()

	for _, eo := range existingOrders {
		key := eo.order.OrderID.String()
		order, err := newOrder(s, eo.order.OrderID, eo.fulfillment)
		if err != nil {
			s.session.Log().Error("creating catchup order", "order", key, "err", err)
			continue
		}
		s.orders[key] = order
	}

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

				key := ev.OrderID.Path()

				s.session.Log().Info("order detected", "order", key)

				if order := s.orders[key]; order != nil {
					s.session.Log().Debug("existing order", "order", key)
					break
				}
				// create an order object for managing the bid process and order lifecycle
				order, err := newOrder(s, ev.OrderID, nil)

				if err != nil {
					// todo: handle error
					s.session.Log().Error("handling order", "order", key, "err", err)
					break
				}

				s.orders[key] = order

			}
		case order := <-s.drainch:
			// child done
			delete(s.orders, order.order.Path())
		}
	}

	// drain: wait for all order monitors to complete.
	for len(s.orders) > 0 {
		delete(s.orders, (<-s.drainch).order.Path())
	}
}

type existingOrder struct {
	order       *types.Order
	fulfillment *types.Fulfillment
}

func queryExistingOrders(ctx context.Context, session session.Session) ([]existingOrder, error) {
	orders, err := session.Query().Orders(ctx)
	if err != nil {
		session.Log().Error("error querying open orders:", "err", err)
		return nil, err
	}

	var existingOrders []existingOrder

	for _, order := range orders.Items {

		if order.State != types.Order_OPEN {
			continue
		}

		eo := existingOrder{order: order}

		eo.fulfillment, _ = session.Query().Fulfillment(ctx, types.FulfillmentID{
			Deployment: order.OrderID.Deployment,
			Group:      order.OrderID.Group,
			Order:      order.OrderID.Seq,
			Provider:   session.Provider().Address,
		})

		existingOrders = append(existingOrders, eo)
	}

	return existingOrders, nil

}
