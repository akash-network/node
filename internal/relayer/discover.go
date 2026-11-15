package relayer

import (
	"context"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

type discovery struct {
	ids   []sink.ID
	known map[sink.ID]sink.TransferState
}

func (s *service) unresolved(ctx context.Context, state native.Snapshot, checkpoint func(context.Context) error) (discovery, error) {
	cursor, err := s.store.loadDiscovery()
	if err != nil {
		return discovery{}, err
	}
	open, err := s.native.Open(ctx, state.Height, checkpoint)
	if err != nil {
		return discovery{}, err
	}
	result := discovery{
		ids:   make([]sink.ID, 0),
		known: make(map[sink.ID]sink.TransferState),
	}
	add := func(transfer sink.TransferState) {
		if _, exists := result.known[transfer.ID]; !exists {
			result.ids = append(result.ids, transfer.ID)
		}
		result.known[transfer.ID] = transfer
	}
	for _, transfer := range open {
		add(transfer)
	}
	unfinished, err := s.store.proofJobIDs()
	if err != nil {
		return discovery{}, err
	}
	ledger, err := s.store.readLedger()
	if err != nil {
		return discovery{}, err
	}
	for _, id := range ledger.ids() {
		entry := ledger.entries[id]
		if entry.Completed == nil || !*entry.Completed {
			unfinished = append(unfinished, id)
		}
	}
	for _, id := range unfinished {
		if checkpoint != nil {
			if err := checkpoint(ctx); err != nil {
				return discovery{}, err
			}
		}
		if _, exists := result.known[id]; exists {
			continue
		}
		transfer, err := s.native.Transfer(ctx, id, state.Height)
		if err != nil {
			return discovery{}, err
		}
		add(transfer)
	}

	discovered := true
	for {
		if checkpoint != nil {
			if err := checkpoint(ctx); err != nil {
				return discovery{}, err
			}
		}
		transfers, next, more, err := s.native.History(ctx, state.Height, cursor)
		if err != nil {
			// Keep open work moving and retry history from the saved checkpoint.
			discovered = false
			s.emit(true, map[string]any{
				"event": "relay-discovery-failed",
				"error": err.Error(),
			})
			break
		}
		for _, transfer := range transfers {
			add(transfer)
		}
		if checkpoint != nil {
			if err := checkpoint(ctx); err != nil {
				return discovery{}, err
			}
		}
		cursor = next
		if !more {
			break
		}
	}
	resolved, err := s.reader.Resolved(ctx, result.ids, checkpoint)
	if err != nil {
		return discovery{}, err
	}
	pending := make([]sink.ID, 0, len(result.ids))
	for i, id := range result.ids {
		if !resolved[i] {
			pending = append(pending, id)
		}
	}
	if discovered {
		if err := s.store.saveDiscovery(cursor); err != nil {
			return discovery{}, err
		}
	}
	result.ids = pending
	return result, nil
}
