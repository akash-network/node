package relayer

import (
	"context"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

func (s *service) prepareProof(ctx context.Context, id sink.ID, height headerHeight) (native.Proof, error) {
	transfer, err := timed(s, "transfer-query", &id, func() (sink.TransferState, error) {
		return s.native.Transfer(ctx, id, int64(height-1))
	})
	if err != nil {
		return native.Proof{}, err
	}
	return timed(s, "proof-query", &id, func() (native.Proof, error) {
		return s.native.Prove(ctx, transfer, int64(height))
	})
}
