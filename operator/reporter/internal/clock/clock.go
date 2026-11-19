// Package clock abstracts time so that timers can be driven deterministically in tests.
package clock

import (
	"sync"
	"time"
)

// Clock is the source of time for the reporter.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

// Real returns a Clock backed by the system clock.
func Real() Clock { return realClock{} }

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Fake is a Clock whose time only moves when Advance is called.
type Fake struct {
	mu      sync.Mutex
	now     time.Time
	waiters []waiter
}

type waiter struct {
	deadline time.Time
	ch       chan time.Time
}

var _ Clock = (*Fake)(nil)

// NewFake returns a Fake clock set to start.
func NewFake(start time.Time) *Fake {
	return &Fake{now: start}
}

// Now returns the fake time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.now
}

// After returns a channel that receives the fake time once it has advanced by d.
// A non-positive d fires immediately.
func (f *Fake) After(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- f.now
		return ch
	}

	f.waiters = append(f.waiters, waiter{deadline: f.now.Add(d), ch: ch})

	return ch
}

// Advance moves the fake time forward by d and fires every timer that became due.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = f.now.Add(d)

	pending := f.waiters[:0]
	for _, w := range f.waiters {
		if w.deadline.After(f.now) {
			pending = append(pending, w)
			continue
		}

		w.ch <- f.now
	}
	f.waiters = pending
}
