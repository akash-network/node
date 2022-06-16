//go:build events_redis

package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/boz/go-lifecycle"
	"github.com/go-redis/redis/v8"
)

type bus struct {
	ctx    context.Context
	client *redis.Client
}

type subscriber struct {
	ctx           context.Context
	cancel        func()
	client        *redis.Client
	lc            lifecycle.Lifecycle
	eventch       chan Event
	subscriptions map[string]bool
}

var _ Bus = (*bus)(nil)

var _ Subscriber = (*subscriber)(nil)

func NewBus() Bus {
	b := &bus{
		ctx: context.Background(),
	}

	b.client = redis.NewClient(&redis.Options{
		Addr: getRedisHost(),
	})

	return b
}

func getRedisHost() string {
	if value, ok := os.LookupEnv("AKASH_EVENTBUS_REDIS"); ok {
		return value
	}
	return "localhost:6379"
}

func newSubscriber(topics ...string) *subscriber {
	b := &subscriber{
		lc:            lifecycle.New(),
		eventch:       make(chan Event),
		subscriptions: make(map[string]bool),
	}

	for _, topic := range topics {
		b.subscriptions[topic] = true
	}

	ctx := context.Background()
	b.ctx, b.cancel = context.WithCancel(ctx)

	b.client = redis.NewClient(&redis.Options{
		Addr: getRedisHost(),
	})

	go b.lc.WatchContext(ctx)
	go b.run()

	return b
}

func (b *subscriber) run() {
	defer func() {
		b.cancel()
		_ = b.client.Close()

		b.lc.ShutdownCompleted()
	}()

	var subs *redis.PubSub

	for topic := range b.subscriptions {
		if subs == nil {
			if strings.Contains(topic, "*") {
				subs = b.client.PSubscribe(b.ctx, topic)
			} else {
				subs = b.client.Subscribe(b.ctx, topic)
			}
		} else {
			var err error
			if strings.Contains(topic, "*") {
				err = subs.PSubscribe(b.ctx, topic)
			} else {
				err = subs.Subscribe(b.ctx, topic)
			}

			if err != nil {

			}
		}
	}

	defer func() {
		_ = subs.Close()
	}()

	subch := subs.Channel()

	var evbuf []Event
	var outch chan<- Event
	var curev Event

loop:
	for {
		if b.eventch != nil && len(evbuf) > 0 {
			// If we're emitting events (Subscriber mode) and there
			// are events to emit, set up the output channel and output
			// event accordingly.
			outch = b.eventch
			curev = evbuf[0]
		} else {
			// otherwise block the output (sending to a nil channel always blocks)
			outch = nil
		}

		select {
		case <-b.lc.ShutdownRequest():
			b.lc.ShutdownInitiated(nil)
			break loop
		case msg := <-subch:
			mType, err := getMessageTypeByTopic(msg.Channel)
			if err != nil {
				fmt.Printf("pubsub error: unknown message %s\n", msg.Channel)
				continue loop
			}

			m := reflect.New(mType.Type())

			err = json.Unmarshal([]byte(msg.Payload), m.Interface())
			if err != nil {
				fmt.Printf("pubsub error: unmarshal message %s\n", err.Error())
				continue loop
			}

			elem := m.Elem().Interface()

			evbuf = append(evbuf, elem)
		case outch <- curev:
			// Event was emitted. Shrink current event buffer.
			evbuf = evbuf[1:]
		}
	}
}

// todo implement with context
func (b *bus) Publish(ev Event) error {
	topic := reflect.TypeOf(ev)

	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}

	cmd := b.client.Publish(b.ctx, topic.String(), data)
	if cmd.Err() != nil {
		return cmd.Err()
	}

	return nil
}

func (b *bus) TopicSubscribe(topic ...string) (Subscriber, error) {
	return b.subscribe(topic...)
}

func (b *bus) subscribe(topics ...string) (*subscriber, error) {
	sub := newSubscriber(topics...)
	return sub, nil
}

func (b *bus) Subscribe() (Subscriber, error) {
	return b.subscribe("*")
}

func (b *bus) Close() {
	_ = b.client.Close()
}

func (b *bus) Done() <-chan struct{} {
	return b.ctx.Done()
}

func (b *subscriber) Clone() (Subscriber, error) {
	topics := make([]string, 0, len(b.subscriptions))

	for topic := range b.subscriptions {
		topics = append(topics, topic)
	}

	sub := newSubscriber(topics...)
	return sub, nil
}

func (b *subscriber) TopicSubscribe(topics ...string) error {
	return nil
}

func (b *subscriber) Events() <-chan Event {
	return b.eventch
}

func (b *subscriber) Close() {
	b.lc.Shutdown(nil)
}

func (b *subscriber) Done() <-chan struct{} {
	return b.lc.Done()
}
