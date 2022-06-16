package pubsub

import (
	"errors"
	"reflect"
	"sync"
)

// ErrNotRunning is the error with message "not running"
var ErrNotRunning = errors.New("not running")
var ErrTypeAlreadyRegistered = errors.New("eventbus: type already registered")
var ErrUnknownMessage = errors.New("eventbus: unknown message")

// Event interface
type Event interface{}

// Bus is an async event bus that allows subscriptions to behave as a bus themselves.
// When an event is published, it is sent to all subscribers asynchronously - a subscriber
// cannot block other subscribers.
//
// NOTE: this should probably be in util/event or something (not in provider/event)
type Bus interface {
	Publish(Event) error
	// Subscribe too all events
	Subscribe() (Subscriber, error)
	TopicSubscribe(topics ...string) (Subscriber, error)
	Close()
	Done() <-chan struct{}
}

// Subscriber emits events it sees on the channel returned by Events().
// A Clone() of a subscriber will emit all events that have not been emitted
// from the cloned subscriber.  This is important so that events are not missed
// when adding subscribers for sub-components (see `provider/bidengine/{service,order}.go`)
type Subscriber interface {
	TopicSubscribe(topics ...string) error
	Events() <-chan Event
	Clone() (Subscriber, error)
	Close()
	Done() <-chan struct{}
}

// ugly
var registeredTypesLock sync.RWMutex
var registeredTypes = map[string]reflect.Value{}

func EventBusRegisterTypes(msgType ...interface{}) error {
	defer registeredTypesLock.Unlock()
	registeredTypesLock.Lock()

	for _, mtype := range msgType {
		msg := reflect.ValueOf(mtype)
		mType := msg.Type().String()
		if _, exists := registeredTypes[mType]; exists {
			return ErrTypeAlreadyRegistered
		}

		registeredTypes[mType] = msg
	}

	return nil
}

func getMessageTypeByTopic(topic string) (reflect.Value, error) {
	msg, exists := registeredTypes[topic]
	if !exists {
		return reflect.Value{}, ErrUnknownMessage
	}

	return msg, nil
}
