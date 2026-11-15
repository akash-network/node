package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	solana "github.com/gagliardetto/solana-go"
)

const (
	expiryPolicy        = "solana-max-processing-age-150-v1"
	expiryMaxAge        = uint64(150)
	expiryWindowBlocks  = 165
	expiryCooldownSize  = 256
	expiryMaxRetryAfter = 2 * time.Hour
)

type expiredHistoryUnavailable struct {
	stage     string
	signature string
	cause     error
}

func (e *expiredHistoryUnavailable) Error() string {
	return fmt.Sprintf("%s: expired transaction history unresolved (%s); durable intent retained; restore signature history and transaction metadata", e.stage, e.signature)
}

func (e *expiredHistoryUnavailable) Unwrap() error { return e.cause }

type expiryIneligibleError struct{}

func (*expiryIneligibleError) Error() string {
	return "top-level System instructions are ineligible for expiry recovery"
}

type intentNotIncludedError struct {
	digest string
}

func (e *intentNotIncludedError) Error() string {
	return "expired intent certified absent and retired; restart from fresh state (certificate " + e.digest + ")"
}

func isIntentNotIncluded(err error) bool {
	var marker *intentNotIncludedError
	return errors.As(err, &marker)
}

type ordinaryIntent struct {
	blockhash string
}

func validateOrdinaryIntent(doc *intent) (ordinaryIntent, error) {
	switch doc.Stage {
	case "proof-create", "proof-grow", "proof-append", "proof-close", "resolve",
		"signature-batch-create", "signature", "chunk", "assembly", "cleanup",
		"lookup-create", "lookup-extend", "lookup-deactivate", "lookup-close":
	default:
		return ordinaryIntent{}, fmt.Errorf("unsupported durable expiry stage")
	}
	if doc.LastValidBlockHeight < int64(expiryMaxAge+12) || doc.LastValidBlockHeight > maxSafeInteger-2 {
		return ordinaryIntent{}, fmt.Errorf("expiry window height outside supported bounds")
	}
	raw, _, err := doc.transaction()
	if err != nil {
		return ordinaryIntent{}, err
	}
	tx, err := solana.TransactionFromBytes(raw)
	if err != nil {
		return ordinaryIntent{}, fmt.Errorf("invalid ordinary transaction")
	}
	if tx.Message.GetVersion() != solana.MessageVersionLegacy && tx.Message.GetVersion() != solana.MessageVersionV0 {
		return ordinaryIntent{}, fmt.Errorf("unsupported ordinary transaction version")
	}
	canonical, err := tx.MarshalBinary()
	if err != nil || !bytes.Equal(canonical, raw) {
		return ordinaryIntent{}, fmt.Errorf("noncanonical ordinary transaction")
	}
	static := len(tx.Message.AccountKeys)
	header := tx.Message.Header
	signers := int(header.NumRequiredSignatures)
	if signers == 0 || signers > static || len(tx.Signatures) != signers || int(header.NumReadonlySignedAccounts) >= signers || int(header.NumReadonlyUnsignedAccounts) > static-signers {
		return ordinaryIntent{}, fmt.Errorf("invalid ordinary transaction header")
	}
	if err := tx.VerifySignatures(); err != nil {
		return ordinaryIntent{}, fmt.Errorf("invalid ordinary transaction signature")
	}
	if tx.Message.RecentBlockhash.IsZero() {
		return ordinaryIntent{}, fmt.Errorf("invalid ordinary transaction blockhash")
	}
	accounts := static
	tables := make(map[solana.PublicKey]bool)
	for _, lookup := range tx.Message.AddressTableLookups {
		if tx.Message.GetVersion() != solana.MessageVersionV0 || lookup.AccountKey.IsZero() || tables[lookup.AccountKey] || len(lookup.WritableIndexes)+len(lookup.ReadonlyIndexes) == 0 {
			return ordinaryIntent{}, fmt.Errorf("invalid ordinary transaction lookup")
		}
		tables[lookup.AccountKey] = true
		indexes := make(map[uint8]bool)
		for _, group := range [][]uint8{lookup.WritableIndexes, lookup.ReadonlyIndexes} {
			for _, index := range group {
				if indexes[index] {
					return ordinaryIntent{}, fmt.Errorf("duplicate ordinary lookup index")
				}
				indexes[index] = true
				accounts++
			}
		}
	}
	if accounts < 1 || accounts > 64 || doc.Accounts != accounts || len(tx.Message.Instructions) == 0 {
		return ordinaryIntent{}, fmt.Errorf("invalid ordinary transaction account count")
	}
	for _, instruction := range tx.Message.Instructions {
		if int(instruction.ProgramIDIndex) >= static {
			return ordinaryIntent{}, fmt.Errorf("ordinary program index outside static accounts")
		}
		if tx.Message.AccountKeys[instruction.ProgramIDIndex] == solana.SystemProgramID {
			return ordinaryIntent{}, &expiryIneligibleError{}
		}
		for _, index := range instruction.Accounts {
			if int(index) >= accounts {
				return ordinaryIntent{}, fmt.Errorf("ordinary instruction account index out of bounds")
			}
		}
	}
	return ordinaryIntent{blockhash: tx.Message.RecentBlockhash.String()}, nil
}

type expiryCooldown struct {
	failures uint
	next     time.Time
}

func (s *service) expiryReady(key string) error {
	s.expiryMu.Lock()
	defer s.expiryMu.Unlock()
	if entry, exists := s.expiryCooldowns[key]; exists && s.deps.now().Before(entry.next) {
		return fmt.Errorf("expired intent certification cooling down; durable intent retained")
	}
	return nil
}

func (s *service) expiryFailed(key string, delay time.Duration) {
	s.expiryMu.Lock()
	defer s.expiryMu.Unlock()
	if s.expiryCooldowns == nil {
		s.expiryCooldowns = make(map[string]expiryCooldown)
	}
	entry, exists := s.expiryCooldowns[key]
	if !exists && len(s.expiryCooldowns) >= expiryCooldownSize {
		var oldest string
		for candidate, value := range s.expiryCooldowns {
			if oldest == "" || value.next.Before(s.expiryCooldowns[oldest].next) {
				oldest = candidate
			}
		}
		delete(s.expiryCooldowns, oldest)
	}
	entry.failures = min(entry.failures+1, 7)
	delay = min(delay, expiryMaxRetryAfter)
	entry.next = s.deps.now().Add(max(delay, min(time.Hour, time.Minute<<(entry.failures-1))))
	s.expiryCooldowns[key] = entry
}

// Certification never observes a transaction or constructs replacement work.
// Its marker unwinds callers with prebuilt instructions before the next pass.
func (p *payerSession) recoverExpired(ctx context.Context, path string, expected *intent) (err error) {
	ctx, cancel := context.WithTimeout(ctx, expiryTimeout)
	defer cancel()
	saved, err := p.service.store.snapshotPending(path)
	if err != nil {
		return err
	}
	if saved.doc != *expected {
		return fmt.Errorf("pending intent changed during settlement; retained")
	}
	if err := p.service.expiryReady(saved.Digest); err != nil {
		return err
	}
	defer func() {
		if err != nil && !isIntentNotIncluded(err) {
			var retry *expiryRPCError
			var delay time.Duration
			if errors.As(err, &retry) {
				delay = retry.retryAfter
			}
			p.service.expiryFailed(saved.Digest, delay)
		}
	}()
	cert, err := p.certifyExpired(ctx, saved)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(cert)
	if err != nil {
		return err
	}
	if err := p.retireExpired(ctx, saved, cert); err != nil {
		return err
	}
	return &intentNotIncludedError{digest: digestHex(encoded)}
}
