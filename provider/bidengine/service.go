package bidengine

import (
	"context"
	"errors"

	lifecycle "github.com/boz/go-lifecycle"

	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// ErrNotRunning declares new error with message "not running"
var ErrNotRunning = errors.New("not running")

// StatusClient interface predefined with Status method
type StatusClient interface {
	Status(context.Context) (*Status, error)
}

// Service handles bidding on orders.
type Service interface {
	StatusClient
	Close() error
	Done() <-chan struct{}
}

// NewService creates new service instance and returns error incase of failure
func NewService(ctx context.Context, session session.Session, cluster cluster.Cluster, bus pubsub.Bus, bps BidPricingStrategy) (Service, error) {
	session = session.ForModule("bidengine-service")

	sub, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}

	existingOrders, err := queryExistingOrders(ctx, session)
	if err != nil {
		session.Log().Error("finding existing orders", "err", err)
		sub.Close()
		return nil, err
	}
	session.Log().Info("found orders", "count", len(existingOrders))

	s := &service{
		session:         session,
		cluster:         cluster,
		bus:             bus,
		sub:             sub,
		statusch:        make(chan chan<- *Status),
		orders:          make(map[string]*order),
		drainch:         make(chan *order),
		lc:              lifecycle.New(),
		pricingStrategy: bps,
	}

	go s.lc.WatchContext(ctx)
	go s.run(existingOrders)

	return s, nil
}

type service struct {
	session session.Session
	cluster cluster.Cluster

	bus pubsub.Bus
	sub pubsub.Subscriber

	statusch        chan chan<- *Status
	orders          map[string]*order
	drainch         chan *order
	pricingStrategy BidPricingStrategy

	lc lifecycle.Lifecycle
}

func (s *service) Close() error {
	s.lc.Shutdown(nil)
	return s.lc.Error()
}

func (s *service) Done() <-chan struct{} {
	return s.lc.Done()
}

func (s *service) Status(ctx context.Context) (*Status, error) {
	ch := make(chan *Status, 1)

	select {
	case <-s.lc.Done():
		return nil, ErrNotRunning
	case <-ctx.Done():
		return nil, ctx.Err()
	case s.statusch <- ch:
	}

	select {
	case <-s.lc.Done():
		return nil, ErrNotRunning
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		return result, nil
	}
}

func (s *service) run(existingOrders []existingOrder) {
	defer s.lc.ShutdownCompleted()
	defer s.sub.Close()

	for _, eo := range existingOrders {
		key := mquery.OrderPath(eo.order.OrderID)
		s.session.Log().Debug("running order", "order", key)
		order, err := newOrder(s, eo.order.OrderID, eo.bid, s.pricingStrategy)
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
			switch ev := ev.(type) { // nolint: gocritic
			case mtypes.EventOrderCreated:
				// new order
				key := mquery.OrderPath(ev.ID)

				s.session.Log().Info("order detected", "order", key)

				if order := s.orders[key]; order != nil {
					s.session.Log().Debug("existing order", "order", key)
					break
				}

				// create an order object for managing the bid process and order lifecycle
				order, err := newOrder(s, ev.ID, nil, s.pricingStrategy)
				if err != nil {
					// todo: handle error
					s.session.Log().Error("handling order", "order", key, "err", err)
					break
				}

				s.orders[key] = order
			}
		case ch := <-s.statusch:
			ch <- &Status{
				Orders: uint32(len(s.orders)),
			}
		case order := <-s.drainch:
			// child done
			key := mquery.OrderPath(order.orderID)
			delete(s.orders, key)
		}
	}

	// drain: wait for all order monitors to complete.
	for len(s.orders) > 0 {
		key := mquery.OrderPath((<-s.drainch).orderID)
		delete(s.orders, key)
	}
}

type existingOrder struct {
	order *mtypes.Order
	bid   *mtypes.Bid
}

func queryExistingOrders(_ context.Context, session session.Session) ([]existingOrder, error) {
	params := &mtypes.QueryOrdersRequest{
		Filters: mtypes.OrderFilters{},
		Pagination: &sdkquery.PageRequest{
			Limit: 10000,
		},
	}
	res, err := session.Client().Query().Orders(context.Background(), params)
	if err != nil {
		session.Log().Error("error querying open orders:", "err", err)
		return nil, err
	}
	orders := res.Orders

	existingOrders := make([]existingOrder, 0)
	for i := range orders {
		pOrder := &orders[i]
		if pOrder.State != mtypes.OrderOpen {
			continue
		}

		eo := existingOrder{order: pOrder}

		res, _ := session.Client().Query().Bid(
			context.Background(),
			&mtypes.QueryBidRequest{
				ID: mtypes.MakeBidID(pOrder.OrderID, session.Provider().Address()),
			},
		)

		bid := res.GetBid()

		eo.bid = &bid

		existingOrders = append(existingOrders, eo)
	}

	return existingOrders, nil

}
