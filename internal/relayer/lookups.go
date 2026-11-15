package relayer

import (
	"context"
	"fmt"
	"math"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

func (s *service) reclaimLookups(ctx context.Context) error {
	err := s.withRequiredPayer(ctx, func(p *payerSession) error {
		return p.reclaimLookups(ctx)
	})
	return err
}

func (p *payerSession) reclaimLookups(ctx context.Context) error {
	s := p.service
	ids, err := s.store.sessionIDs()
	if err != nil {
		return err
	}
	if err := p.checkGenesis(ctx); err != nil {
		return err
	}

	closed := 0
	for _, id := range ids {
		if closed >= 1 {
			break
		}

		doc, err := s.store.loadSession(id)
		if err != nil {
			return err
		}
		if doc == nil {
			return fmt.Errorf("lookup session disappeared during cleanup")
		}
		if doc.Lookup == nil {
			continue
		}

		address, err := solana.PublicKeyFromBase58(*doc.Lookup)
		if err != nil {
			return err
		}
		path, err := s.store.sessionClosePendingPath(id)
		if err != nil {
			return err
		}

		observer := func(result txResult) error {
			if transactionFailed(result) {
				doc.LookupCloseFailure = transactionDocument(result)
				return s.store.saveSession(id, doc)
			}
			if doc.LookupRentClosing == nil {
				return fmt.Errorf("lookup close has no saved rent")
			}

			value := true
			doc.LookupClosed = &value
			doc.LookupClosureUnobserved = nil
			doc.LookupRentReturned = doc.LookupRentClosing
			doc.LookupCloseTransaction = &result.signature
			if err := s.store.saveSession(id, doc); err != nil {
				return err
			}

			closed++
			s.emit(false, map[string]any{
				"event":     "lookup-rent-returned",
				"address":   address.String(),
				"lamports":  *doc.LookupRentClosing,
				"signature": result.signature,
			})
			return nil
		}

		if _, err := p.resume(ctx, path, observer); err != nil {
			return err
		}
		unobserved := doc.LookupClosureUnobserved != nil && *doc.LookupClosureUnobserved
		// Older sessions marked an absent table closed before creation landed.
		// Only an observed closure is terminal; preserve its receipt if present.
		if doc.LookupClosed != nil && *doc.LookupClosed && (!unobserved || doc.LookupCloseTransaction != nil) {
			continue
		}

		table, err := s.reader.Lookup(ctx, address)
		if err != nil {
			return err
		}
		if table == nil {
			if doc.LookupClosed == nil && unobserved {
				continue
			}
			value := true
			doc.LookupClosed = nil
			doc.LookupClosureUnobserved = &value
			if err := s.store.saveSession(id, doc); err != nil {
				return err
			}
			continue
		}

		if table.State.Authority == nil || *table.State.Authority != s.payer.PublicKey() {
			return fmt.Errorf("lookup authority mismatch")
		}
		if unobserved {
			doc.LookupClosed = nil
			doc.LookupClosureUnobserved = nil
			if err := s.store.saveSession(id, doc); err != nil {
				return err
			}
		}
		if table.State.DeactivationSlot == math.MaxUint64 {
			continue
		}

		slots, err := s.reader.SlotHashes(ctx)
		if err != nil {
			return err
		}
		if slots[len(slots)-1] <= table.State.DeactivationSlot {
			continue
		}

		before, err := s.headerAccount(ctx, address, rpc.CommitmentConfirmed)
		if err != nil {
			return err
		}
		if before == nil || before.Lamports > maxSafeInteger {
			return fmt.Errorf("lookup rent unavailable or exceeds safe integer")
		}

		rent := int64(before.Lamports)
		doc.LookupRentClosing = &rent
		if err := s.store.saveSession(id, doc); err != nil {
			return err
		}

		ix, err := bridge.CloseLookup(address, s.payer.PublicKey(), s.payer.PublicKey())
		if err != nil {
			return err
		}
		result, err := p.send(ctx, txRequest{
			stage:        "lookup-close",
			instructions: []solana.Instruction{ix},
		}, path, observer)
		if err != nil {
			return err
		}

		if !transactionFailed(result) {
			after, err := s.headerAccount(ctx, address, rpc.CommitmentFinalized)
			if err != nil {
				return err
			}
			if after != nil {
				return fmt.Errorf("closed lookup account remains")
			}
		}
	}
	return nil
}
