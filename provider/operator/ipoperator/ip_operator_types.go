package ipoperator

import (
	"context"
	"github.com/ovrclk/akash/manifest/v2beta1"
	"github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"sync/atomic"
	"time"
)

/*
	Types that are used only within the IP Address operator locally
*/

type managedIP struct {
	presentLease        mtypes.LeaseID
	presentServiceName  string
	lastEvent           v1beta2.IPResourceEvent
	presentSharingKey   string
	presentExternalPort uint32
	presentPort         uint32
	lastChangedAt       time.Time
	presentProtocol     v2beta1.ServiceProtocol
}

type barrier struct {
	enabled int32
	active  int32
}

func (b *barrier) enable() {
	atomic.StoreInt32(&b.enabled, 1)
}

func (b *barrier) disable() {
	atomic.StoreInt32(&b.enabled, 0)
}

func (b *barrier) enter() bool {
	isEnabled := atomic.LoadInt32(&b.enabled) == 1
	if !isEnabled {
		return false
	}

	atomic.AddInt32(&b.active, 1)
	return true
}

func (b *barrier) exit() {
	atomic.AddInt32(&b.active, -1)
}

func (b *barrier) waitUntilClear(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			clear := 0 == atomic.LoadInt32(&b.active)
			if clear {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
