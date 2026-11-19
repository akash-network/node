package solana

import (
	"errors"
	"fmt"
)

// TransferResolved is the event a record's resolution emits; it carries the same evidence as TransferStatus.
type TransferResolved TransferStatus

// DecodeTransferResolved decodes the Anchor event data, discriminator included. The length must be exact.
func DecodeTransferResolved(data []byte) (TransferResolved, error) {
	r := reader{data: data}
	r.discriminator(DiscTransferResolved)

	event := r.transferStatus()
	if r.err == nil && len(r.data) != 0 {
		r.fail(errors.New("trailing bytes"))
	}

	if r.err != nil {
		return TransferResolved{}, fmt.Errorf("decode TransferResolved: %w", r.err)
	}

	return TransferResolved(event), nil
}
