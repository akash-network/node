package testutil

import (
	"reflect"
	"testing"
	"time"
)

func ChannelWaitForValueUpTo(t *testing.T, waitOn interface{}, waitFor time.Duration) interface{} {
	cases := make([]reflect.SelectCase, 2)
	cases[0] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(waitOn),
		Send: reflect.Value{},
	}

	delayChan := time.After(waitFor)

	cases[1] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(delayChan),
		Send: reflect.Value{},
	}

	idx, v, ok := reflect.Select(cases)
	if !ok {
		t.Fatal("Channel has been closed")
	}
	if idx != 0 {
		t.Fatalf("No message after waiting %v seconds", waitFor)
	}

	return v.Interface()
}

func ChannelWaitForValue(t *testing.T, waitOn interface{}) interface{} {
	const waitForDefault = 10 * time.Second
	return ChannelWaitForValueUpTo(t, waitOn, waitForDefault)
}
