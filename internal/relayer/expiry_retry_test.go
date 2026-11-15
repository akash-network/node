package relayer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExpiryRetryAfterMaximum(t *testing.T) {
	for _, header := range []string{"4294967295", "Wed, 01 Jan 2100 00:00:00 GMT"} {
		t.Run(header, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			f.reply = func(w http.ResponseWriter, id json.RawMessage, method string) bool {
				if method != "getBlock" {
					return false
				}
				w.Header().Set("Retry-After", header)
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":429,"message":"limited"}}`, id)
				return true
			}
			f.retained(context.Background())
			saved, err := f.service.store.snapshotPending(f.path)
			require.NoError(t, err)
			cooldown := f.service.expiryCooldowns[saved.Digest]
			require.Equal(t, 2*time.Hour, cooldown.next.Sub(f.service.deps.now()))
			f.reply = nil
			f.mu.Lock()
			f.now = cooldown.next.Add(-time.Millisecond)
			f.mu.Unlock()
			blocks := f.calls["getBlock"]
			require.ErrorContains(t, f.retained(context.Background()), "cooling down")
			require.Equal(t, blocks, f.calls["getBlock"])
			f.mu.Lock()
			f.now = cooldown.next
			f.mu.Unlock()
			require.True(t, isIntentNotIncluded(f.resume(context.Background())))
		})
	}
}
