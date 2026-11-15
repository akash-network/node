package native

import (
	"bytes"
	"context"
	"errors"

	"github.com/cosmos/cosmos-sdk/types/query"

	sink "pkg.akt.dev/go/node/sink/v1"
)

type Cursor struct {
	Kind   string
	Key    []byte
	LastID sink.ID
}

type page struct {
	transfers []sink.TransferState
	next      []byte
}

func (c *Client) page(ctx context.Context, height int64, key []byte, status sink.Status, disputes bool) (page, error) {
	pagination := &query.PageRequest{
		Key:   key,
		Limit: 500,
	}

	var result page
	if disputes {
		reply, err := c.queries(height).Disputes(ctx, &sink.QueryDisputesRequest{Pagination: pagination})
		if err != nil {
			return page{}, err
		}
		if reply.Pagination == nil {
			return page{}, errors.New("missing pagination")
		}
		result.transfers, result.next = reply.Transfers, reply.Pagination.NextKey
	} else {
		reply, err := c.queries(height).Transfers(ctx, &sink.QueryTransfersRequest{
			Pagination: pagination,
			Status:     status,
		})
		if err != nil {
			return page{}, err
		}
		if reply.Pagination == nil {
			return page{}, errors.New("missing pagination")
		}
		result.transfers, result.next = reply.Transfers, reply.Pagination.NextKey
	}

	if len(result.transfers) > 500 {
		return page{}, errors.New("invalid transfers page length")
	}
	for _, transfer := range result.transfers {
		if err := validTransfer(transfer); err != nil {
			return page{}, err
		}
		if disputes && transfer.Settlement.Hold == sink.HoldNone {
			return page{}, errors.New("unheld dispute")
		}
		if !disputes && status != sink.StatusInvalid && transfer.Status != status {
			return page{}, errors.New("status filter mismatch")
		}
	}

	return result, nil
}

func (c *Client) collect(ctx context.Context, height int64, status sink.Status, disputes bool, checkpoint func(context.Context) error) ([]sink.TransferState, error) {
	result := make([]sink.TransferState, 0)
	seen := make(map[sink.ID]bool)
	cursors := map[string]bool{"": true}
	var key []byte

	for {
		if checkpoint != nil {
			if err := checkpoint(ctx); err != nil {
				return nil, err
			}
		}
		page, err := c.page(ctx, height, key, status, disputes)
		if err != nil {
			return nil, err
		}

		for _, transfer := range page.transfers {
			if seen[transfer.ID] {
				return nil, errors.New("duplicate transfer in paginated query")
			}
			seen[transfer.ID] = true
			result = append(result, transfer)
		}

		if checkpoint != nil {
			if err := checkpoint(ctx); err != nil {
				return nil, err
			}
		}

		if len(page.next) == 0 {
			return result, nil
		}
		if len(page.transfers) == 0 || cursors[string(page.next)] {
			return nil, errors.New("pagination cursor did not advance")
		}
		cursors[string(page.next)] = true
		key = page.next
	}
}

func (c *Client) Open(ctx context.Context, height int64, checkpoint func(context.Context) error) ([]sink.TransferState, error) {
	result := make([]sink.TransferState, 0)
	positions := make(map[sink.ID]int)

	for _, status := range []sink.Status{sink.StatusInvalid, sink.StatusRefundPending, sink.StatusVoided, sink.StatusTransferPending} {
		transfers, err := c.collect(ctx, height, status, status == sink.StatusInvalid, checkpoint)
		if err != nil {
			return nil, err
		}

		for _, transfer := range transfers {
			if position, found := positions[transfer.ID]; found {
				result[position] = transfer
			} else {
				positions[transfer.ID] = len(result)
				result = append(result, transfer)
			}
		}
	}

	return result, nil
}

// History returns one chronological page. A failure never returns an advanced cursor.
func (c *Client) History(ctx context.Context, height int64, after *Cursor) ([]sink.TransferState, *Cursor, bool, error) {
	var key []byte
	var last sink.ID
	if after != nil {
		if after.Kind != "next" && after.Kind != "tail" {
			return nil, nil, false, errors.New("invalid history cursor kind")
		}
		if _, err := ParseID(after.LastID.String()); err != nil {
			return nil, nil, false, err
		}
		key, last = after.Key, after.LastID
	}

	page, err := c.page(ctx, height, key, sink.StatusInvalid, false)
	if err != nil {
		return nil, nil, false, err
	}
	if after != nil && after.Kind == "next" && len(page.transfers) == 0 {
		return nil, nil, false, errors.New("chronological checkpoint disappeared")
	}

	found := after == nil || after.Kind != "tail"
	result := make([]sink.TransferState, 0)
	var previous sink.ID
	for _, transfer := range page.transfers {
		id := transfer.ID
		if previous.Height != 0 && compareIDs(id, previous) <= 0 {
			return nil, nil, false, errors.New("history is not chronological")
		}
		previous = id

		if after != nil && after.Kind == "tail" && compareIDs(id, after.LastID) <= 0 {
			if id == after.LastID {
				found = true
			}
			continue
		}

		if !found {
			return nil, nil, false, errors.New("chronological checkpoint disappeared")
		}
		if last.Height != 0 && compareIDs(id, last) <= 0 {
			return nil, nil, false, errors.New("history is not chronological")
		}
		result = append(result, transfer)
		last = id
	}
	if !found {
		return nil, nil, false, errors.New("chronological checkpoint disappeared")
	}

	more := len(page.next) > 0
	var cursor *Cursor
	if more {
		if last.Height == 0 || len(page.transfers) == 0 || bytes.Equal(key, page.next) {
			return nil, nil, false, errors.New("pagination cursor did not advance")
		}
		cursor = &Cursor{
			Kind:   "next",
			Key:    bytes.Clone(page.next),
			LastID: last,
		}
	} else if last.Height != 0 {
		cursor = &Cursor{
			Kind:   "tail",
			Key:    bytes.Clone(key),
			LastID: last,
		}
	}

	return result, cursor, more, nil
}
