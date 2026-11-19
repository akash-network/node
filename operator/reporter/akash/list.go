package akash

import (
	"bytes"
	"context"
	"errors"

	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// The listing texts, verbatim from contracts/solana/reporter/akash.ts and contracts/solana/local/open-transfers.ts.
var (
	errPageLimit         = errors.New("invalid page limit")
	errTransfersHeight   = errors.New("Akash transfers query height mismatch") //nolint:staticcheck // verbatim TS text
	errDisputesHeight    = errors.New("Akash disputes query height mismatch")  //nolint:staticcheck // verbatim TS text
	errTransfersPage     = errors.New("invalid transfers page")
	errDisputesPage      = errors.New("invalid disputes page")
	errMissingSettlement = errors.New("missing settlement")
	errStatusFilter      = errors.New("status filter mismatch")
	errUnheldDispute     = errors.New("unheld dispute")
	errDuplicate         = errors.New("duplicate transfer in paginated query")
	errCursorStalled     = errors.New("pagination cursor did not advance")
	errCheckpoint        = errors.New("chronological checkpoint disappeared")
	errNotChronological  = errors.New("history is not chronological")
)

// CursorKind is how the audit resumes.
type CursorKind int

const (
	// CursorNext resumes at the server's next page.
	CursorNext CursorKind = iota + 1
	// CursorTail replays the last page, skipping IDs up to LastID, to find transfers added since.
	CursorTail
)

// AuditCursor is where the chronological audit resumes: the server's opaque page key and the last ID returned.
type AuditCursor struct {
	Kind   CursorKind
	Key    []byte
	LastID sinkid.ID
}

// AuditPage is one page of the chronological audit.
type AuditPage struct {
	IDs     []sinkid.ID
	Cursor  *AuditCursor
	HasMore bool
}

// source is one listing: the held disputes, a status bucket, or the chronological history when status is unset.
type source struct {
	disputes bool
	status   sinkv1.Status
}

// openSources are the open listings in priority order: disputes, then refund-pending, void-requested and
// transfer-pending. Disputes and refunds come first because their veto and payment deadlines are the most urgent.
var openSources = []source{
	{disputes: true},
	{status: sinkv1.StatusRefundPending},
	{status: sinkv1.StatusVoided},
	{status: sinkv1.StatusTransferPending},
}

// page is one decoded listing page.
type page struct {
	ids     []sinkid.ID
	nextKey []byte
}

// ListOpen lists every disputed, refund-pending, void-requested and transfer-pending transfer at height. The four
// listings run concurrently, each paging sequentially, and merge in that order; an ID listed twice keeps its first
// position.
func (c *Client) ListOpen(ctx context.Context, height int64, pageSize int) ([]sinkid.ID, error) {
	lists := make([][]sinkid.ID, len(openSources))

	g, gctx := errgroup.WithContext(ctx)
	for i, src := range openSources {
		g.Go(func() error {
			var err error
			lists[i], err = collectPages(gctx, func(ctx context.Context, key []byte) (page, error) {
				return c.readPage(ctx, height, src, key, pageSize)
			})

			return err
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return merge(lists...), nil
}

// Audit reads one page of the chronological history at height from after, which is nil at the start, as
// discoverTransfers does.
func (c *Client) Audit(ctx context.Context, height int64, after *AuditCursor, limit int) (AuditPage, error) {
	var key []byte
	if after != nil {
		key = after.Key
	}

	p, err := c.readPage(ctx, height, source{}, key, limit)
	if err != nil {
		return AuditPage{}, err
	}

	return discover(after, p)
}

// readPage reads and checks one page of src at height.
func (c *Client) readPage(ctx context.Context, height int64, src source, key []byte, limit int) (page, error) {
	if limit <= 0 {
		return page{}, errPageLimit
	}

	req := &sdkquery.PageRequest{Key: key, Limit: uint64(limit)}

	var (
		transfers  []sinkv1.TransferState
		pagination *sdkquery.PageResponse
		err        error
	)

	if src.disputes {
		err = c.pinned(ctx, height, errDisputesHeight, func(ctx context.Context, opts ...grpc.CallOption) error {
			res, err := c.sink.Disputes(ctx, &sinkv1.QueryDisputesRequest{Pagination: req}, opts...)
			if err == nil {
				transfers, pagination = res.Transfers, res.Pagination
			}

			return err
		})
	} else {
		err = c.pinned(ctx, height, errTransfersHeight, func(ctx context.Context, opts ...grpc.CallOption) error {
			res, err := c.sink.Transfers(ctx, &sinkv1.QueryTransfersRequest{Status: src.status, Pagination: req}, opts...)
			if err == nil {
				transfers, pagination = res.Transfers, res.Pagination
			}

			return err
		})
	}

	if err != nil {
		return page{}, err
	}

	return decodePage(src, limit, transfers, pagination)
}

// decodePage applies the TS page checks: pagination present, at most limit entries, each a valid record with a
// settlement, matching the status filter, and held when it is a dispute.
func decodePage(src source, limit int, transfers []sinkv1.TransferState, pagination *sdkquery.PageResponse) (page, error) {
	if pagination == nil || len(transfers) > limit {
		if src.disputes {
			return page{}, errDisputesPage
		}

		return page{}, errTransfersPage
	}

	ids := make([]sinkid.ID, 0, len(transfers))
	for _, t := range transfers {
		rec, err := sinkid.DecodeRecord(t)
		if err != nil {
			return page{}, err
		}

		switch {
		case src.disputes:
			if t.Settlement.Hold == sinkv1.HoldNone {
				return page{}, errUnheldDispute
			}
		case isZero(t.Settlement):
			return page{}, errMissingSettlement
		case src.status != sinkv1.StatusInvalid && t.Status != src.status:
			return page{}, errStatusFilter
		}

		ids = append(ids, rec.ID)
	}

	return page{ids: ids, nextKey: pagination.NextKey}, nil
}

// collectPages reads every page from the first, as collectPages in open-transfers.ts does: no ID may appear twice,
// and each cursor must be new and follow a non-empty page. Cursor bytes are opaque and need not increase.
func collectPages(ctx context.Context, read func(context.Context, []byte) (page, error)) ([]sinkid.ID, error) {
	var (
		out     []sinkid.ID
		key     []byte
		seen    = map[string]struct{}{}
		cursors = map[string]struct{}{"": {}}
	)

	for {
		p, err := read(ctx, key)
		if err != nil {
			return nil, err
		}

		for _, id := range p.ids {
			k := string(id.KeySuffix())
			if _, ok := seen[k]; ok {
				return nil, errDuplicate
			}

			seen[k] = struct{}{}
			out = append(out, id)
		}

		if len(p.nextKey) == 0 {
			return out, nil
		}

		if _, ok := cursors[string(p.nextKey)]; ok || len(p.ids) == 0 {
			return nil, errCursorStalled
		}

		cursors[string(p.nextKey)] = struct{}{}
		key = p.nextKey
	}
}

// discover applies the chronological checks and cursor rules of discoverTransfers to a history page read from after.
func discover(after *AuditCursor, p page) (AuditPage, error) {
	if after != nil && after.Kind == CursorNext && len(p.ids) == 0 {
		return AuditPage{}, errCheckpoint
	}

	var (
		last     *sinkid.ID
		previous *sinkid.ID
		out      []sinkid.ID
	)

	if after != nil {
		last = &after.LastID
	}

	tail := after != nil && after.Kind == CursorTail
	found := !tail

	for i := range p.ids {
		id := p.ids[i]
		if previous != nil && sinkid.Compare(id, *previous) <= 0 {
			return AuditPage{}, errNotChronological
		}
		previous = &p.ids[i]

		if tail && sinkid.Compare(id, after.LastID) <= 0 {
			if sinkid.Compare(id, after.LastID) == 0 {
				found = true
			}

			continue
		}

		if !found {
			return AuditPage{}, errCheckpoint
		}

		if last != nil && sinkid.Compare(id, *last) <= 0 {
			return AuditPage{}, errNotChronological
		}

		out = append(out, id)
		last = &p.ids[i]
	}

	if !found {
		return AuditPage{}, errCheckpoint
	}

	result := AuditPage{IDs: out, HasMore: len(p.nextKey) > 0}

	var afterKey []byte
	if after != nil {
		afterKey = after.Key
	}

	switch {
	case result.HasMore:
		if last == nil || len(p.ids) == 0 || bytes.Equal(p.nextKey, afterKey) {
			return AuditPage{}, errCursorStalled
		}

		result.Cursor = &AuditCursor{Kind: CursorNext, Key: bytes.Clone(p.nextKey), LastID: *last}
	case last != nil:
		// Replay only the last page on the next poll. The server owns its cursor; the last ID tells already-audited
		// rows from later deposits.
		result.Cursor = &AuditCursor{Kind: CursorTail, Key: bytes.Clone(afterKey), LastID: *last}
	}

	return result, nil
}

// merge concatenates lists, keeping each ID at its first position.
func merge(lists ...[]sinkid.ID) []sinkid.ID {
	var out []sinkid.ID
	seen := map[string]struct{}{}

	for _, list := range lists {
		for _, id := range list {
			k := string(id.KeySuffix())
			if _, ok := seen[k]; ok {
				continue
			}

			seen[k] = struct{}{}
			out = append(out, id)
		}
	}

	return out
}
