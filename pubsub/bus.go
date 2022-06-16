//go:build !events_zeromq && !events_redis

package pubsub

import (
	"errors"

	"github.com/boz/go-lifecycle"
)

type bus struct {
	subscriptions map[*bus]bool

	evbuf []Event

	eventch  chan Event
	parentch chan *bus

	pubch   chan Event
	subch   chan chan<- Subscriber
	unsubch chan *bus

	lc lifecycle.Lifecycle
}

// NewBus runs a new bus and returns bus details
func NewBus() Bus {
	bus := &bus{
		subscriptions: make(map[*bus]bool),
		pubch:         make(chan Event),
		subch:         make(chan chan<- Subscriber),
		unsubch:       make(chan *bus),
		lc:            lifecycle.New(),
	}

	go bus.run()

	return bus
}

func (b *bus) Publish(ev Event) error {
	select {
	case b.pubch <- ev:
		return nil
	case <-b.lc.ShuttingDown():
		return ErrNotRunning
	}
}

func (b *bus) Subscribe() (Subscriber, error) {
	ch := make(chan Subscriber, 1)

	select {
	case b.subch <- ch:
		return <-ch, nil
	case <-b.lc.ShuttingDown():
		return nil, ErrNotRunning
	}
}

func (b *bus) Clone() (Subscriber, error) {
	return b.Subscribe()
}

func (b *bus) Events() <-chan Event {
	return b.eventch
}

func (b *bus) Close() {
	b.lc.Shutdown(nil)
}

func (b *bus) Done() <-chan struct{} {
	return b.lc.Done()
}

func (b *bus) run() {
	defer b.lc.ShutdownCompleted()

	var outch chan<- Event
	var curev Event

loop:
	for {

		if b.eventch != nil && len(b.evbuf) > 0 {
			// If we're emitting events (Subscriber mode) and there
			// are events to emit, set up the output channel and output
			// event accordingly.
			outch = b.eventch
			curev = b.evbuf[0]
		} else {
			// otherwise block the output (sending to a nil channel always blocks)
			outch = nil
		}

		select {
		case err := <-b.lc.ShutdownRequest():
			b.lc.ShutdownInitiated(err)
			break loop

		case outch <- curev:
			// Event was emitted. Shrink current event buffer.
			b.evbuf = b.evbuf[1:]

		case ev := <-b.pubch:
			// publish event

			// Buffer event.
			if b.eventch != nil {
				b.evbuf = append(b.evbuf, ev)
			}

			// Publish to children.
			for sub := range b.subscriptions {
				if err := sub.Publish(ev); err != nil && !errors.Is(err, ErrNotRunning) {
					panic(err)
				}
			}

		case ch := <-b.subch:
			// new subscription

			sub := newSubscriber(b)
			b.subscriptions[sub] = true

			ch <- sub

		case sub := <-b.unsubch:
			// subscription closed
			delete(b.subscriptions, sub)
		}
	}

	for sub := range b.subscriptions {
		sub.lc.ShutdownAsync(nil)
	}

	for len(b.subscriptions) > 0 {
		sub := <-b.unsubch
		delete(b.subscriptions, sub)
	}

	if b.parentch != nil {
		b.parentch <- b
	}
}

func newSubscriber(parent *bus) *bus {
	// Re-use bus struct, but populate output channel (eventch)
	// to enable subscriber mode.

	evbuf := make([]Event, len(parent.evbuf))
	copy(evbuf, parent.evbuf)

	sub := &bus{
		eventch:  make(chan Event),
		parentch: parent.unsubch,
		evbuf:    evbuf,

		subscriptions: make(map[*bus]bool),
		pubch:         make(chan Event),
		subch:         make(chan chan<- Subscriber),
		unsubch:       make(chan *bus),
		lc:            lifecycle.New(),
	}

	go sub.run()

	return sub
}
