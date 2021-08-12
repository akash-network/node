package bidengine

import (
	"context"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/boz/go-lifecycle"

	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

var (
	ordersCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "provider_order_handler",
		Help: "The total number of orders created",
	}, []string{"action"})
)

// ErrNotRunning declares new error with message "not running"
var ErrNotRunning = errors.New("not running")

// StatusClient interface predefined with Status method
type StatusClient interface {
	Status(context.Context) (*Status, error)
}

var (
	orderManagerGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "provider_order_manager",
		Help:        "",
		ConstLabels: nil,
	})
)

// Service handles bidding on orders.
type Service interface {
	StatusClient
	Close() error
	Done() <-chan struct{}
}

// NewService creates new service instance and returns error in case of failure
func NewService(ctx context.Context, session session.Session, cluster cluster.Cluster, bus pubsub.Bus, cfg Config) (Service, error) {
	session = session.ForModule("bidengine-service")

	existingOrders, err := queryExistingOrders(ctx, session)
	if err != nil {
		session.Log().Error("finding existing orders", "err", err)
		return nil, err
	}

	sub, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}

	session.Log().Info("found orders", "count", len(existingOrders))

	providerAttrService, err := newProviderAttrSignatureService(session, bus)
	if err != nil {
		return nil, err
	}

	s := &service{
		session:  session,
		cluster:  cluster,
		bus:      bus,
		sub:      sub,
		statusch: make(chan chan<- *Status),
		orders:   make(map[string]*order),
		drainch:  make(chan *order),
		lc:       lifecycle.New(),
		cfg:      cfg,
		pass:     providerAttrService,
	}

	go s.lc.WatchContext(ctx)
	go s.run(existingOrders)

	return s, nil
}

type service struct {
	session session.Session
	cluster cluster.Cluster
	cfg     Config

	bus pubsub.Bus
	sub pubsub.Subscriber

	statusch chan chan<- *Status
	orders   map[string]*order
	drainch  chan *order

	lc   lifecycle.Lifecycle
	pass *providerAttrSignatureService
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

func (s *service) updateOrderManagerGauge() {
	orderManagerGauge.Set(float64(len(s.orders)))
}

func (s *service) run(existingOrders []mtypes.OrderID) {
	defer s.lc.ShutdownCompleted()
	defer s.sub.Close()
	s.updateOrderManagerGauge()
	for _, orderID := range existingOrders {
		key := mquery.OrderPath(orderID)
		s.session.Log().Debug("creating catchup order", "order", key)
		order, err := newOrder(s, orderID, s.cfg, s.pass, true)
		if err != nil {
			s.session.Log().Error("creating catchup order", "order", key, "err", err)
			continue
		}
		s.orders[key] = order
		s.updateOrderManagerGauge()
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
				order, err := newOrder(s, ev.ID, s.cfg, s.pass, false)
				if err != nil {
					s.session.Log().Error("handling order", "order", key, "err", err)
					break
				}

				ordersCounter.WithLabelValues("start").Inc()
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
			ordersCounter.WithLabelValues("stop").Inc()
		}
		s.updateOrderManagerGauge()
	}

	s.pass.lc.ShutdownAsync(nil)

	s.session.Log().Info("draining order monitors", "qty", len(s.orders))
	// drain: wait for all order monitors to complete.
	for len(s.orders) > 0 {
		key := mquery.OrderPath((<-s.drainch).orderID)
		delete(s.orders, key)
		s.updateOrderManagerGauge()
	}

	s.session.Log().Info("Waiting on provider attributes service")
	<-s.pass.lc.Done()
}

func queryExistingOrders(ctx context.Context, session session.Session) ([]mtypes.OrderID, error) {
	params := &mtypes.QueryOrdersRequest{
		Filters: mtypes.OrderFilters{},
		Pagination: &sdkquery.PageRequest{
			Limit: 10000,
		},
	}
	res, err := session.Client().Query().Orders(ctx, params)
	if err != nil {
		session.Log().Error("error querying open orders:", "err", err)
		return nil, err
	}
	orders := res.Orders

	existingOrders := make([]mtypes.OrderID, 0)
	for i := range orders {
		pOrder := &orders[i]
		// Only check existing orders that are open
		if pOrder.State != mtypes.OrderOpen {
			continue
		}
		existingOrders = append(existingOrders, pOrder.OrderID)
	}

	return existingOrders, nil
}
