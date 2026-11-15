package relayer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

type intent struct {
	Stage                string `json:"stage"`
	Genesis              string `json:"genesis"`
	Payer                string `json:"payer"`
	Raw                  string `json:"raw"`
	Signature            string `json:"signature"`
	LastValidBlockHeight int64  `json:"lastValidBlockHeight"`
	Accounts             int    `json:"accounts"`
}

func (s *stateStore) loadIntent(path string) (*intent, error) {
	data, err := optionalJSON(path)
	if err != nil || data == nil {
		return nil, err
	}
	fields, err := jsonObject(data)
	if err != nil {
		return nil, err
	}
	var doc intent
	if err := requireFields(fields, documentFields(doc)...); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if err := s.validateIntent(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *stateStore) validateIntent(doc *intent) error {
	if doc == nil || doc.Genesis != s.identity.Genesis || doc.Payer != s.identity.Payer {
		return fmt.Errorf("transaction intent belongs to another deployment or payer")
	}
	if !positiveSafe(doc.LastValidBlockHeight) || doc.Accounts < 1 || doc.Accounts > 64 {
		return fmt.Errorf("invalid transaction intent height or account count")
	}
	_, _, err := doc.transaction()
	return err
}

func (s *stateStore) saveIntent(path string, doc *intent) error {
	if err := s.validateIntent(doc); err != nil {
		return err
	}
	return s.save(path, doc)
}

func (s *stateStore) removeIntent(path string) error { return os.Remove(path) }

func (doc *intent) transaction() ([]byte, solana.Signature, error) {
	raw, err := base64.StdEncoding.DecodeString(doc.Raw)
	if err != nil || len(raw) > 1232 || base64.StdEncoding.EncodeToString(raw) != doc.Raw {
		return nil, solana.Signature{}, fmt.Errorf("invalid signed transaction encoding or size")
	}
	tx, err := solana.TransactionFromBytes(raw)
	if err != nil {
		return nil, solana.Signature{}, fmt.Errorf("invalid signed transaction: %w", err)
	}
	if len(tx.Signatures) == 0 || len(tx.Message.AccountKeys) == 0 || tx.Signatures[0].String() != doc.Signature || tx.Message.AccountKeys[0].String() != doc.Payer {
		return nil, solana.Signature{}, fmt.Errorf("transaction signature or payer mismatch")
	}
	return raw, tx.Signatures[0], nil
}

func finalizedStage(stage string) bool {
	return strings.HasPrefix(stage, "lookup-") || stage == "assembly" || stage == "cleanup" || strings.HasPrefix(stage, "proof-") || stage == "resolve"
}

func (p *payerSession) checkGenesis(ctx context.Context) error {
	genesis, err := p.service.solana.GetGenesisHash(ctx)
	if err != nil {
		return err
	}
	if genesis.String() != p.service.genesis || genesis.String() != p.service.store.identity.Genesis {
		return fmt.Errorf("Solana genesis changed; refusing paid transaction")
	}
	return nil
}

func (p *payerSession) settle(ctx context.Context, doc *intent, submitted func(string) error) (txResult, error) {
	if err := p.checkGenesis(ctx); err != nil {
		return txResult{}, err
	}
	if doc.Genesis != p.service.genesis || doc.Payer != p.service.payer.PublicKey().String() {
		return txResult{}, fmt.Errorf("transaction intent belongs to another deployment or payer")
	}
	raw, signature, err := doc.transaction()
	if err != nil {
		return txResult{}, err
	}
	deadline := p.service.deps.now().Add(90 * time.Second)
	var resendAt time.Time
	for {
		statuses, err := p.service.solana.GetSignatureStatuses(ctx, true, signature)
		if err != nil {
			return txResult{}, err
		}
		if statuses == nil || len(statuses.Value) != 1 {
			return txResult{}, fmt.Errorf("invalid signature status response; durable intent retained")
		}
		status := statuses.Value[0]
		confirmed := status != nil && (status.ConfirmationStatus == rpc.ConfirmationStatusConfirmed || status.ConfirmationStatus == rpc.ConfirmationStatusFinalized)
		if confirmed && (!finalizedStage(doc.Stage) || status.ConfirmationStatus == rpc.ConfirmationStatusFinalized) {
			break
		}
		if status == nil {
			height, err := p.service.solana.GetBlockHeight(ctx, rpc.CommitmentFinalized)
			if err != nil {
				return txResult{}, err
			}
			if height > uint64(doc.LastValidBlockHeight) {
				unresolved := fmt.Sprintf("%s: expired transaction history unresolved (%s); durable intent retained; restore signature history and transaction metadata", doc.Stage, doc.Signature)
				result, err := p.finalizedHistory(ctx, doc)
				if err != nil {
					return txResult{}, fmt.Errorf("%s: %w", unresolved, err)
				}
				if result != nil {
					return *result, nil
				}
				if err := p.explicitExpiredStatus(ctx, doc); err != nil {
					return txResult{}, fmt.Errorf("%s: %w", unresolved, err)
				}
				return txResult{}, &expiredHistoryUnavailable{
					stage:     doc.Stage,
					signature: doc.Signature,
				}
			}
		}
		now := p.service.deps.now()
		if !now.Before(deadline) {
			return txResult{}, fmt.Errorf("%s: confirmation unresolved (%s); durable intent retained", doc.Stage, doc.Signature)
		}
		if !confirmed && !now.Before(resendAt) {
			zeroRetries := uint(0)
			options := rpc.TransactionOpts{
				SkipPreflight: true,
				MaxRetries:    &zeroRetries,
				Encoding:      solana.EncodingBase64,
			}
			if doc.Stage == "bootstrap" {
				options.SkipPreflight = false
				options.PreflightCommitment = rpc.CommitmentConfirmed
				options.MaxRetries = nil
			}
			_, err := p.service.solana.SendEncodedTransactionWithOpts(ctx, doc.Raw, options)
			if err != nil {
				return txResult{}, err
			}
			if submitted != nil {
				if err := submitted(doc.Signature); err != nil {
					return txResult{}, err
				}
			}
			resendAt = p.service.deps.now().Add(2 * time.Second)
		}
		if err := p.service.deps.sleep(ctx, 150*time.Millisecond); err != nil {
			return txResult{}, err
		}
	}

	version := uint64(0)
	var result *rpc.GetTransactionResult
	for attempt := 0; attempt <= 20; attempt++ {
		result, err = p.service.solana.GetTransaction(ctx, signature, &rpc.GetTransactionOpts{
			Commitment:                     rpc.CommitmentConfirmed,
			MaxSupportedTransactionVersion: &version,
		})
		if err != nil && !errors.Is(err, rpc.ErrNotFound) {
			return txResult{}, err
		}
		if result != nil || attempt == 20 {
			break
		}
		if err := p.service.deps.sleep(ctx, 100*time.Millisecond); err != nil {
			return txResult{}, err
		}
	}
	if result == nil || result.Meta == nil {
		return txResult{}, fmt.Errorf("missing transaction metadata: %s", doc.Signature)
	}
	meta := result.Meta
	if len(meta.PreBalances) == 0 || len(meta.PostBalances) == 0 {
		return txResult{}, fmt.Errorf("missing payer balance metadata: %s", doc.Signature)
	}
	var delta int64
	if meta.PreBalances[0] >= meta.PostBalances[0] {
		change := meta.PreBalances[0] - meta.PostBalances[0]
		if change > uint64(maxSafeInteger) {
			return txResult{}, fmt.Errorf("payer balance change exceeds safe integer")
		}
		delta = int64(change)
	} else {
		change := meta.PostBalances[0] - meta.PreBalances[0]
		if change > uint64(maxSafeInteger) {
			return txResult{}, fmt.Errorf("payer balance change exceeds safe integer")
		}
		delta = -int64(change)
	}
	failure, err := json.Marshal(meta.Err)
	if err != nil {
		return txResult{}, err
	}
	compute := uint64(0)
	if meta.ComputeUnitsConsumed != nil {
		compute = *meta.ComputeUnitsConsumed
	}
	if meta.Fee > uint64(maxSafeInteger) || compute > uint64(maxSafeInteger) {
		return txResult{}, fmt.Errorf("transaction metadata exceeds safe integer")
	}
	logs := meta.LogMessages
	if logs == nil {
		logs = []string{}
	}
	return txResult{
		stage:         doc.Stage,
		signature:     doc.Signature,
		bytes:         len(raw),
		accounts:      doc.Accounts,
		fee:           meta.Fee,
		compute:       compute,
		failure:       failure,
		logs:          logs,
		payerLamports: &delta,
	}, nil
}

func (p *payerSession) resume(ctx context.Context, path string, observer observe) (*txResult, error) {
	doc, err := p.service.store.loadIntent(path)
	if err != nil || doc == nil {
		return nil, err
	}
	if observer == nil {
		return nil, fmt.Errorf("transaction observer is required")
	}
	result, err := p.settle(ctx, doc, nil)
	if err != nil {
		var expired *expiredHistoryUnavailable
		if errors.As(err, &expired) {
			recovery := p.recoverExpired(ctx, path, doc)
			if isIntentNotIncluded(recovery) {
				return nil, recovery
			}
			var ineligible *expiryIneligibleError
			if errors.As(recovery, &ineligible) {
				expired.cause = recovery
				return nil, expired
			}
			return nil, fmt.Errorf("%w; expiry certification: %w", expired, recovery)
		}
		return nil, err
	}
	if err := p.reconcileExpiryReceipt(path, doc, result); err != nil {
		return nil, err
	}
	if err := observer(result); err != nil {
		return nil, err
	}
	if err := p.service.store.removeIntent(path); err != nil {
		return nil, err
	}
	return &result, nil
}

func (p *payerSession) send(ctx context.Context, request txRequest, pendingPath string, observer observe) (txResult, error) {
	if observer == nil {
		return txResult{}, fmt.Errorf("transaction observer is required")
	}
	if pendingPath != "" {
		pending, err := optionalJSON(pendingPath)
		if err != nil {
			return txResult{}, err
		}
		if pending != nil {
			return txResult{}, fmt.Errorf("resolve previous transaction before submitting another")
		}
	}
	if err := p.checkGenesis(ctx); err != nil {
		return txResult{}, err
	}
	hash, err := p.service.solana.GetLatestBlockhash(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return txResult{}, err
	}
	if hash == nil || hash.Value == nil || hash.Value.LastValidBlockHeight == 0 || hash.Value.LastValidBlockHeight > uint64(maxSafeInteger) {
		return txResult{}, fmt.Errorf("invalid latest blockhash response")
	}
	tx, accounts, err := bridge.Compile(p.service.payer.PublicKey(), hash.Value.Blockhash, request.instructions, request.tables)
	if err != nil {
		return txResult{}, err
	}
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key == p.service.payer.PublicKey() {
			return &p.service.payer
		}
		return nil
	})
	if err != nil {
		return txResult{}, err
	}
	raw, err := tx.MarshalBinary()
	if err != nil {
		return txResult{}, err
	}
	if len(raw) > 1232 {
		return txResult{}, fmt.Errorf("%s: %d bytes exceeds 1232", request.stage, len(raw))
	}
	doc := &intent{
		Stage:                request.stage,
		Genesis:              p.service.genesis,
		Payer:                p.service.payer.PublicKey().String(),
		Raw:                  base64.StdEncoding.EncodeToString(raw),
		Signature:            tx.Signatures[0].String(),
		LastValidBlockHeight: int64(hash.Value.LastValidBlockHeight),
		Accounts:             accounts,
	}
	if request.preflight {
		simulation, err := p.service.solana.SimulateTransactionWithOpts(ctx, tx, &rpc.SimulateTransactionOpts{
			SigVerify:  true,
			Commitment: rpc.CommitmentConfirmed,
		})
		if err != nil {
			return txResult{}, err
		}
		if simulation == nil || simulation.Value == nil {
			return txResult{}, fmt.Errorf("%s: missing simulation result", request.stage)
		}
		if simulation.Value.Err != nil {
			failure, err := json.Marshal(simulation.Value.Err)
			if err != nil {
				return txResult{}, fmt.Errorf("%s: encode simulation failure: %w", request.stage, err)
			}
			return txResult{}, fmt.Errorf("%s: simulation failed: %s", request.stage, failure)
		}
	}
	if pendingPath != "" {
		if err := p.service.store.saveIntent(pendingPath, doc); err != nil {
			return txResult{}, err
		}
	}
	result, err := p.settle(ctx, doc, request.submitted)
	if err != nil {
		return txResult{}, err
	}
	if err := observer(result); err != nil {
		return txResult{}, err
	}
	if pendingPath != "" {
		if err := p.service.store.removeIntent(pendingPath); err != nil {
			return txResult{}, err
		}
	}
	return result, nil
}

func (p *payerSession) sendPrune(ctx context.Context, instructions []solana.Instruction) (txResult, error) {
	return p.send(ctx, txRequest{
		stage:        "consensus-prune",
		instructions: instructions,
	}, "", func(txResult) error { return nil })
}

// Bootstrap follows Anchor's confirmed send with RPC preflight, without an
// intent or a separate simulation. Retries keep that same preflight policy.
func (p *payerSession) sendBootstrap(ctx context.Context, instructions []solana.Instruction) (txResult, error) {
	result, err := p.send(ctx, txRequest{
		stage:        "bootstrap",
		instructions: instructions,
	}, "", func(txResult) error { return nil })
	if err != nil {
		return txResult{}, err
	}
	if transactionFailed(result) {
		return result, fmt.Errorf("bootstrap failed: %s (%s)", result.failure, result.signature)
	}
	return result, nil
}

func transactionFailed(result txResult) bool {
	return len(result.failure) != 0 && !bytes.Equal(bytes.TrimSpace(result.failure), []byte("null"))
}
