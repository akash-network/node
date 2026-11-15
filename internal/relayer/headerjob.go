package relayer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/header"
)

const headerChunkSize = 900
const signatureBatchCapacity = 128
const maxSignatureBatches = 2

func (s *service) headerPacket(ctx context.Context, trusted, target headerHeight, revision uint64, trust *header.Fraction) (header.Packet, string, error) {
	source, err := s.native.HeaderSource(ctx, int64(trusted), int64(target), revision)
	if err != nil {
		return header.Packet{}, "", err
	}
	if target <= trusted+1 {
		trust = nil
	}
	selected, report, err := header.Select(source, header.Selection{
		Trigger: s.cfg.maxSignatures,
		Limit:   signatureBatchCapacity,
		Trust:   trust,
	})
	if err != nil {
		return header.Packet{}, "", err
	}
	if report.Kept < report.Present {
		s.emit(false, map[string]any{
			"event":   "header-signatures-trimmed",
			"target":  target,
			"present": report.Present,
			"kept":    report.Kept,
		})
	}

	encoded, err := header.Encode(selected)
	if err != nil {
		return header.Packet{}, "", err
	}
	if encoded.ChainID != s.cfg.chainID || encoded.Height != uint64(target) {
		return header.Packet{}, "", fmt.Errorf("encoded header chain or height mismatch")
	}
	packet, err := header.RuntimePacket(encoded)
	return packet, hex.EncodeToString(encoded.BlockHash[:]), err
}

func packetDocument(chainID string, packet header.Packet) headerPacketDoc {
	signatures := make([]headerSignatureDoc, len(packet.Receipts))
	for i, receipt := range packet.Receipts {
		signatures[i] = headerSignatureDoc{
			Hash:      hex.EncodeToString(receipt.Hash[:]),
			Pubkey:    hex.EncodeToString(receipt.PubKey[:]),
			Message:   hex.EncodeToString(receipt.Message),
			Signature: hex.EncodeToString(receipt.Signature[:]),
		}
	}

	return headerPacketDoc{
		ChainID: chainID,
		Header:  hex.EncodeToString(packet.Header),
		Consensus: headerConsensusDoc{
			Timestamp:          strconv.FormatUint(packet.Timestamp, 10),
			Root:               hex.EncodeToString(packet.Root[:]),
			NextValidatorsHash: hex.EncodeToString(packet.NextValidatorsHash[:]),
		},
		Signatures: signatures,
	}
}

func runtimeHeaderPacket(raw json.RawMessage, chainID string) (header.Packet, error) {
	doc, err := decodeActivePacket(raw, chainID)
	if err != nil {
		return header.Packet{}, err
	}
	var packet header.Packet
	packet.Header, err = hex.DecodeString(doc.Header)
	if err != nil {
		return header.Packet{}, err
	}
	packet.Timestamp, err = strconv.ParseUint(doc.Consensus.Timestamp, 10, 64)
	if err != nil {
		return header.Packet{}, err
	}

	root, err := hex.DecodeString(doc.Consensus.Root)
	if err != nil {
		return header.Packet{}, err
	}
	next, err := hex.DecodeString(doc.Consensus.NextValidatorsHash)
	if err != nil {
		return header.Packet{}, err
	}

	copy(packet.Root[:], root)
	copy(packet.NextValidatorsHash[:], next)
	packet.Receipts = make([]header.Receipt, len(doc.Signatures))
	for i, sig := range doc.Signatures {
		hash, err := hex.DecodeString(sig.Hash)
		if err != nil {
			return header.Packet{}, err
		}
		key, err := hex.DecodeString(sig.Pubkey)
		if err != nil {
			return header.Packet{}, err
		}
		signature, err := hex.DecodeString(sig.Signature)
		if err != nil {
			return header.Packet{}, err
		}
		packet.Receipts[i].Message, err = hex.DecodeString(sig.Message)
		if err != nil {
			return header.Packet{}, err
		}
		copy(packet.Receipts[i].Hash[:], hash)
		copy(packet.Receipts[i].PubKey[:], key)
		copy(packet.Receipts[i].Signature[:], signature)
	}
	return packet, nil
}

type headerAdmissionError struct {
	cause error
}

func (e *headerAdmissionError) Error() string { return e.cause.Error() }

func (e *headerAdmissionError) Unwrap() error { return e.cause }

// Scheduled commands bind their own follow to the tick's identity and current
// update rights. Standalone commands retain their original follow behavior.
func (s *service) followHeader(ctx context.Context, expected *bridge.Followed) (bridge.Followed, error) {
	found, err := s.follow(ctx)
	if expected == nil {
		return found, err
	}
	if err == nil && (found.Program != expected.Program || found.AccessManager != expected.AccessManager || found.ChainID != expected.ChainID) {
		err = fmt.Errorf("light client identity changed during tick")
	}
	if err == nil {
		var allowed bridge.Access
		allowed, err = s.relayerAccess(ctx, found)
		if err == nil && allowed == bridge.AccessNone {
			err = fmt.Errorf("header update access revoked during tick")
		}
	}
	if err != nil {
		return bridge.Followed{}, &headerAdmissionError{cause: err}
	}
	return found, nil
}

func (s *service) resumeHeader(ctx context.Context, expected *bridge.Followed) error {
	pending, err := s.store.loadActive()
	if err != nil || pending == nil {
		return err
	}

	followed, err := s.followHeader(ctx, expected)
	if err != nil {
		return err
	}

	err = s.withRequiredPayer(ctx, func(p *payerSession) error {
		pending, err := s.store.loadActive()
		if err != nil || pending == nil {
			return err
		}
		return p.finishHeader(ctx, pending, followed)
	})
	return err
}

func (s *service) update(ctx context.Context, target headerHeight, tamper bool, expected *bridge.Followed) error {
	if tamper && !s.cfg.local {
		return fmt.Errorf("header tampering is only available to the local stack")
	}
	if !positiveSafe(int64(target)) {
		return fmt.Errorf("invalid header target")
	}

	followed, err := s.followHeader(ctx, expected)
	if err != nil {
		return err
	}

	err = s.withRequiredPayer(ctx, func(p *payerSession) error {
		pending, err := s.store.loadActive()
		if err != nil {
			return err
		}
		if pending != nil {
			if err := p.finishHeader(ctx, pending, followed); err != nil {
				return err
			}
		}

		current, err := s.reader.Client(ctx, followed)
		if err != nil {
			return err
		}
		if current.LatestHeight.RevisionHeight > maxSafeInteger {
			return fmt.Errorf("trusted height exceeds safe integer")
		}
		trusted := headerHeight(current.LatestHeight.RevisionHeight)
		if target <= trusted {
			return nil
		}
		if err := s.reader.Active(ctx, followed); err != nil {
			return err
		}

		packet, _, err := s.headerPacket(ctx, trusted, target, current.LatestHeight.RevisionNumber, &header.Fraction{
			Numerator:   current.TrustLevelNumerator,
			Denominator: current.TrustLevelDenominator,
		})
		if err != nil {
			return err
		}
		if tamper {
			offset := bytes.Index(packet.Header, packet.Root[:])
			if offset < 0 {
				return fmt.Errorf("header does not contain application root")
			}
			packet.Header[offset] ^= 1
		}

		raw, err := json.Marshal(packetDocument(s.cfg.chainID, packet))
		if err != nil {
			return err
		}
		job := &activeHeaderDoc{
			ChainID: s.cfg.chainID,
			Genesis: s.genesis,
			Client:  followed.Program.String(),
			Target:  int64(target),
			Trusted: int64(trusted),
			Packet:  raw,
		}
		if err := s.store.saveActive(job); err != nil {
			return err
		}
		return p.finishHeader(ctx, job, followed)
	})
	return err
}

func (p *payerSession) finishHeader(ctx context.Context, job *activeHeaderDoc, followed bridge.Followed) error {
	s := p.service
	if err := s.store.validateActive(job); err != nil {
		return err
	}
	if job.Client != followed.Program.String() {
		saved, err := s.store.abandonActive(headerHeight(job.Target), s.deps.now())
		if err != nil {
			return err
		}
		s.emit(true, map[string]any{
			"event":   "header-upload-abandoned",
			"client":  job.Client,
			"current": followed.Program.String(),
			"target":  job.Target,
			"saved":   saved,
		})
		return nil
	}

	packet, err := runtimeHeaderPacket(job.Packet, s.cfg.chainID)
	if err != nil {
		return err
	}
	result, err := p.uploadHeader(ctx, followed, packet, headerHeight(job.Target), headerHeight(job.Trusted), signatureBatchCapacity)
	if err != nil {
		return err
	}
	if err := s.store.removeActive(); err != nil {
		return err
	}

	var failure map[string]json.RawMessage
	if json.Unmarshal(result.assembly.failure, &failure) == nil {
		if reason, ok := failure["unsupportedHeader"]; ok {
			var message string
			if err := json.Unmarshal(reason, &message); err != nil {
				message = string(reason)
			}
			return &header.UnsupportedError{
				Reason: message,
			}
		}
	}
	if transactionFailed(result.assembly) {
		return fmt.Errorf("UpdateClientFailed: %s", result.assembly.failure)
	}

	s.emit(false, map[string]any{
		"event":        "authenticated-header",
		"trusted":      job.Trusted,
		"height":       job.Target,
		"signatures":   result.commitSignatures,
		"bytes":        result.headerBytes,
		"computeUnits": result.assembly.compute,
		"signature":    result.assembly.signature,
	})
	return nil
}

type headerLayout struct {
	session   [32]byte
	sessions  [][32]byte
	chunks    []solana.PublicKey
	batches   []solana.PublicKey
	addresses []solana.PublicKey
	assembly  solana.Instruction
}

func layoutHeader(f bridge.Followed, payer solana.PublicKey, packet header.Packet, target, trusted headerHeight, perBatch int) (headerLayout, string, error) {
	if perBatch < 1 || perBatch > signatureBatchCapacity {
		return headerLayout{}, "", fmt.Errorf("invalid signatures per batch")
	}
	count := (len(packet.Header) + headerChunkSize - 1) / headerChunkSize
	batchCount := max(1, (len(packet.Receipts)+perBatch-1)/perBatch)
	if count < 1 || count > 255 {
		return headerLayout{}, fmt.Sprintf("header is %d bytes; the uploader supports 1 to 255 chunks of 900 bytes", len(packet.Header)), nil
	}
	if batchCount > maxSignatureBatches {
		return headerLayout{}, fmt.Sprintf("commit has %d signatures; the verifier accepts at most 256 preverified signatures per header. Extend the ICS07 signature-batch patch before the validator set grows past this", len(packet.Receipts)), nil
	}

	layout := headerLayout{
		sessions: make([][32]byte, batchCount),
		chunks:   make([]solana.PublicKey, count),
		batches:  make([]solana.PublicKey, batchCount),
	}

	hash := sha256.New()
	hash.Write([]byte("akt-header-upload-v1"))
	hash.Write(f.Program[:])
	hash.Write(payer[:])
	hash.Write(packet.Header)
	copy(layout.session[:], hash.Sum(nil))

	for k := range layout.sessions {
		layout.sessions[k] = layout.session
		if k != 0 {
			hash.Reset()
			hash.Write([]byte("akt-header-upload-batch-v1"))
			hash.Write(layout.session[:])
			hash.Write([]byte{byte(k)})
			copy(layout.sessions[k][:], hash.Sum(nil))
		}
		address, err := f.BatchAt(payer, layout.sessions[k])
		if err != nil {
			return headerLayout{}, "", err
		}
		layout.batches[k] = address
	}

	for i := range layout.chunks {
		address, err := f.ChunkAt(payer, uint64(target), uint8(i))
		if err != nil {
			return headerLayout{}, "", err
		}
		layout.chunks[i] = address
	}

	assembly, err := f.Assemble(payer, uint64(target), uint64(trusted), layout.chunks, layout.batches)
	if err != nil {
		return headerLayout{}, "", err
	}
	layout.assembly = assembly

	seen := make(map[solana.PublicKey]bool)
	add := func(key solana.PublicKey) {
		if !seen[key] {
			seen[key] = true
			layout.addresses = append(layout.addresses, key)
		}
	}

	for _, meta := range assembly.Accounts() {
		add(meta.PublicKey)
	}
	add(f.Program)
	add(solana.ComputeBudget)
	if len(layout.addresses) > 64 {
		return headerLayout{}, fmt.Sprintf("assembly needs %d accounts; conservative limit is 64", len(layout.addresses)), nil
	}
	return layout, "", nil
}

type headerUploadResult struct {
	headerBytes              int
	chunks                   int
	commitSignatures         int
	preverified              int
	signaturesWithoutReceipt int
	assembly                 txResult
	transactions             int
	maxTransactionBytes      int
	totalCompute             uint64
	fees                     uint64
	temporaryRentReturned    uint64
	permanentRent            uint64
	lookupRent               uint64
	balanceDelta             int64
	lookupTable              *solana.PublicKey
}

func assemblyExhaustion(result txResult, program solana.PublicKey) string {
	if !transactionFailed(result) {
		return ""
	}
	prefix := "Program " + program.String() + " failed: "
	for _, line := range result.logs {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		if strings.Contains(line, "exceeded CUs meter") || strings.Contains(line, "Computational budget exceeded") {
			return "assembly exhausted the compute budget"
		}
		if strings.Contains(line, "heap section") || strings.Contains(line, "out of memory") {
			return "assembly exhausted the heap"
		}
	}
	return ""
}

func matchingConsensus(stored *bridge.Consensus, packet header.Packet) bool {
	return stored.Timestamp == packet.Timestamp && stored.Root == packet.Root && stored.NextValidatorsHash == packet.NextValidatorsHash
}

// Rent observations need no account codec. Only the RPC's explicit absence is nil.
func (s *service) headerAccount(ctx context.Context, address solana.PublicKey, commitment rpc.CommitmentType) (*rpc.Account, error) {
	account, _, err := s.reader.Account(ctx, address, commitment, nil)
	return account, err
}

func (s *service) headerAccounts(ctx context.Context, addresses []solana.PublicKey, commitment rpc.CommitmentType) ([]*rpc.Account, error) {
	accounts, _, err := s.reader.Accounts(ctx, addresses, commitment)
	return accounts, err
}

func (p *payerSession) uploadHeader(ctx context.Context, f bridge.Followed, packet header.Packet, target, trusted headerHeight, perBatch int) (result headerUploadResult, err error) {
	s := p.service
	payer := s.payer.PublicKey()
	result.headerBytes = len(packet.Header)
	result.chunks = (len(packet.Header) + headerChunkSize - 1) / headerChunkSize
	result.commitSignatures = len(packet.Receipts)
	layout, reason, err := layoutHeader(f, payer, packet, target, trusted, perBatch)
	if err != nil {
		return result, err
	}
	if reason != "" {
		if err := s.store.appendHeaderEvent(map[string]any{
			"event":  "unsupported-header",
			"target": target,
			"reason": reason,
		}); err != nil {
			return result, err
		}
		failure, _ := json.Marshal(map[string]string{"unsupportedHeader": reason})
		result.assembly = txResult{
			stage:   "unsupported",
			failure: failure,
			logs:    []string{},
		}
		result.signaturesWithoutReceipt = len(packet.Receipts)
		return result, nil
	}

	before, err := s.solana.GetBalance(ctx, payer, rpc.CommitmentConfirmed)
	if err != nil {
		return result, err
	}
	if before == nil {
		return result, fmt.Errorf("missing payer balance")
	}
	if err := p.checkGenesis(ctx); err != nil {
		return result, err
	}

	id := hex.EncodeToString(layout.session[:])
	doc, err := s.store.loadSession(id)
	if err != nil {
		return result, err
	}
	if doc == nil {
		doc = &sessionDoc{
			Version: 1,
			Genesis: s.genesis,
			Payer:   payer.String(),
			Client:  f.Program.String(),
			Target:  int64(target),
			Trusted: int64(trusted),
			Session: id,
		}
	} else if doc.Client != f.Program.String() || doc.Target != int64(target) || doc.Trusted != int64(trusted) {
		return result, fmt.Errorf("upload journal identity mismatch")
	}

	if doc.Assembly != nil {
		failure := bytes.TrimSpace(doc.Assembly.Error)
		if len(failure) != 0 && !bytes.Equal(failure, []byte("null")) && failure[0] != '{' && failure[0] != '[' {
			return result, fmt.Errorf("invalid terminal assembly error")
		}
		// The original restart path deliberately discards terminal metrics and logs.
		doc.Assembly = transactionDocument(txResult{
			stage:     "assembly",
			signature: doc.Assembly.Signature,
			failure:   doc.Assembly.Error,
			logs:      []string{},
		})
	}

	if err := s.store.saveSession(id, doc); err != nil {
		return result, err
	}

	path, err := s.store.sessionPendingPath(id)
	if err != nil {
		return result, err
	}

	var transactions []txResult
	observer := func(entry txResult) error {
		transactions = append(transactions, entry)
		if entry.stage == "assembly" {
			doc.Assembly = transactionDocument(entry)
			if err := s.store.saveSession(id, doc); err != nil {
				return err
			}
		}

		data, err := json.Marshal(transactionDocument(entry))
		if err != nil {
			return err
		}
		event, err := jsonObject(data)
		if err != nil {
			return err
		}
		event["event"] = json.RawMessage(`"transaction"`)
		return s.store.appendHeaderEvent(event)
	}

	if _, err := p.resume(ctx, path, observer); err != nil {
		return result, err
	}

	already, err := s.reader.Consensus(ctx, f, uint64(target))
	if err != nil {
		return result, err
	}
	if already != nil && !matchingConsensus(already, packet) {
		return result, fmt.Errorf("stored target differs from upload; refusing to skip verification")
	}

	var lookup *bridge.Lookup
	send := func(stage string, instructions []solana.Instruction, useLookup bool) (txResult, error) {
		request := txRequest{
			stage:        stage,
			instructions: instructions,
			submitted: func(signature string) error {
				return s.store.appendHeaderEvent(map[string]any{
					"event":     "submitted",
					"stage":     stage,
					"signature": signature,
				})
			},
		}
		if useLookup && lookup != nil {
			request.tables = map[solana.PublicKey]solana.PublicKeySlice{lookup.Address: lookup.State.Addresses}
		}
		entry, err := p.send(ctx, request, path, observer)
		if err == nil && stage != "assembly" && transactionFailed(entry) {
			err = fmt.Errorf("%s failed: %s", stage, entry.failure)
		}
		return entry, err
	}

	remaining := append(append([]solana.PublicKey{}, layout.chunks...), layout.batches...)
	var assembly *txResult
	// As in the TS finally block, unknown outcomes retain every temporary account.
	defer func() {
		if assembly != nil {
			if cleanupErr := p.cleanupHeader(ctx, f, remaining, doc.Lookup, send); cleanupErr != nil {
				err = cleanupErr
			}
		}
	}()

	if already == nil && doc.Assembly == nil {
		if err := s.reader.Active(ctx, f); err != nil {
			return result, err
		}
		lookup, err = p.prepareHeaderLookup(ctx, id, doc, layout.addresses, send)
		if err != nil {
			return result, err
		}

		for k, address := range layout.batches {
			batch, err := s.reader.Batch(ctx, f, payer, layout.sessions[k])
			if err != nil {
				return result, err
			}
			if batch == nil {
				ix, err := f.InitializeSignatureBatch(payer, layout.sessions[k])
				if err != nil {
					return result, err
				}
				if _, err := send("signature-batch-create", []solana.Instruction{ix}, false); err != nil {
					return result, err
				}
				batch, err = s.reader.Batch(ctx, f, payer, layout.sessions[k])
				if err != nil {
					return result, err
				}
			}
			if batch == nil {
				return result, fmt.Errorf("signature batch unavailable after creation")
			}

			hashes := make(map[[32]byte]bool)
			for _, hash := range batch.Hashes {
				hashes[hash] = true
			}

			for i := k * perBatch; i < min(len(packet.Receipts), (k+1)*perBatch); i++ {
				receipt := packet.Receipts[i]
				if hashes[receipt.Hash] {
					continue
				}
				verify, err := bridge.Ed25519(receipt)
				if err != nil {
					return result, err
				}
				ix, err := f.PreVerifySignatureBatch(payer, address, receipt)
				if err != nil {
					return result, err
				}
				if _, err := send("signature", []solana.Instruction{verify, ix}, false); err != nil {
					return result, err
				}
			}
		}

		for i := range layout.chunks {
			data := packet.Header[i*headerChunkSize : min(len(packet.Header), (i+1)*headerChunkSize)]
			existing, err := s.reader.Chunk(ctx, f, payer, uint64(target), uint8(i))
			if err != nil {
				return result, err
			}
			if existing != nil {
				if !bytes.Equal(existing.Data, data) {
					return result, fmt.Errorf("existing chunk differs from durable header")
				}
				continue
			}
			ix, err := f.UploadHeaderChunk(payer, uint64(target), uint8(i), data)
			if err != nil {
				return result, err
			}
			if _, err := send("chunk", []solana.Instruction{ix}, true); err != nil {
				return result, err
			}
		}

		temporary, err := s.headerAccounts(ctx, remaining, rpc.CommitmentConfirmed)
		if err != nil {
			return result, err
		}
		for _, account := range temporary {
			if account != nil && !bridge.HeaderTemporaryInitializable(account) {
				result.temporaryRentReturned += account.Lamports
			}
		}

		entry, err := send("assembly", append(bridge.Compute(), layout.assembly), true)
		if err != nil {
			return result, err
		}
		assembly = &entry
	} else {
		entry := txResult{stage: "already-authenticated"}
		if already == nil {
			entry = doc.Assembly.result()
		}
		assembly = &entry
		if err := s.store.appendHeaderEvent(map[string]any{
			"event":  "already-authenticated",
			"target": target,
		}); err != nil {
			return result, err
		}
	}

	if exhausted := assemblyExhaustion(*assembly, f.Program); exhausted != "" {
		if err := s.store.appendHeaderEvent(map[string]any{
			"event":  "unsupported-header",
			"target": target,
			"reason": exhausted,
		}); err != nil {
			return result, err
		}
		assembly.failure, _ = json.Marshal(map[string]any{
			"unsupportedHeader": exhausted,
			"cause":             json.RawMessage(assembly.failure),
		})
	}

	state, err := s.reader.ClientFinalized(ctx, f)
	if err != nil {
		return result, err
	}
	stored, err := s.reader.Consensus(ctx, f, uint64(target))
	if err != nil {
		return result, err
	}
	if stored != nil {
		if state.LatestHeight.RevisionHeight < uint64(target) || !matchingConsensus(stored, packet) {
			return result, fmt.Errorf("authenticated target differs from requested header")
		}
		if transactionFailed(*assembly) {
			if err := s.store.appendHeaderEvent(map[string]any{
				"event":         "authenticated-concurrently",
				"target":        target,
				"localAssembly": assembly.signature,
			}); err != nil {
				return result, err
			}
			assembly.stage = "authenticated-concurrently"
			assembly.failure = nil
		}
	} else if !transactionFailed(*assembly) {
		return result, fmt.Errorf("successful finalized assembly has no authenticated target")
	}

	// Cleanup must finish before the post-upload rent and balance observations.
	if err := p.cleanupHeader(ctx, f, remaining, doc.Lookup, send); err != nil {
		assembly = nil
		return result, err
	}
	result.assembly = *assembly
	assembly = nil
	result.preverified = len(packet.Receipts)

	targetAddress, err := f.ConsensusAt(uint64(target))
	if err != nil {
		return result, err
	}
	account, err := s.headerAccount(ctx, targetAddress, rpc.CommitmentConfirmed)
	if err != nil {
		return result, err
	}
	if account != nil {
		result.permanentRent = account.Lamports
	}

	if doc.Lookup != nil {
		address, err := solana.PublicKeyFromBase58(*doc.Lookup)
		if err != nil {
			return result, err
		}
		result.lookupTable = &address
		account, err := s.headerAccount(ctx, address, rpc.CommitmentConfirmed)
		if err != nil {
			return result, err
		}
		if account != nil {
			result.lookupRent = account.Lamports
		}
	}

	result.transactions = len(transactions)
	for _, entry := range transactions {
		result.fees += entry.fee
		result.totalCompute += entry.compute
		result.maxTransactionBytes = max(result.maxTransactionBytes, entry.bytes)
	}

	after, err := s.solana.GetBalance(ctx, payer, rpc.CommitmentConfirmed)
	if err != nil {
		return result, err
	}
	if after == nil || before.Value > math.MaxInt64 || after.Value > math.MaxInt64 {
		return result, fmt.Errorf("invalid payer balance")
	}
	result.balanceDelta = int64(before.Value) - int64(after.Value)
	return result, nil
}

type headerSender func(string, []solana.Instruction, bool) (txResult, error)

func (p *payerSession) prepareHeaderLookup(ctx context.Context, id string, doc *sessionDoc, addresses []solana.PublicKey, send headerSender) (*bridge.Lookup, error) {
	s := p.service
	payer := s.payer.PublicKey()
	allocate := func() error {
		slot, err := s.solana.GetSlot(ctx, rpc.CommitmentFinalized)
		if err != nil {
			return err
		}
		if slot > maxSafeInteger {
			return fmt.Errorf("lookup slot exceeds safe integer")
		}
		_, address, err := bridge.CreateLookup(payer, payer, slot)
		if err != nil {
			return err
		}
		text, number := address.String(), int64(slot)
		doc.Lookup, doc.LookupSlot = &text, &number
		return s.store.saveSession(id, doc)
	}

	if doc.Lookup == nil {
		if err := allocate(); err != nil {
			return nil, err
		}
	}

	address, err := solana.PublicKeyFromBase58(*doc.Lookup)
	if err != nil {
		return nil, err
	}

	lookup, err := s.reader.LookupConfirmed(ctx, address)
	if err != nil {
		return nil, err
	}

	if lookup == nil {
		slot, err := s.solana.GetSlot(ctx, rpc.CommitmentFinalized)
		if err != nil {
			return nil, err
		}
		if slot > maxSafeInteger {
			return nil, fmt.Errorf("lookup slot exceeds safe integer")
		}
		if int64(slot)-*doc.LookupSlot > 512 {
			if err := allocate(); err != nil {
				return nil, err
			}
			address, err = solana.PublicKeyFromBase58(*doc.Lookup)
			if err != nil {
				return nil, err
			}
		}
		if *doc.LookupSlot < 0 {
			return nil, fmt.Errorf("invalid lookup creation slot")
		}
		ix, _, err := bridge.CreateLookup(payer, payer, uint64(*doc.LookupSlot))
		if err != nil {
			return nil, err
		}
		if _, err := send("lookup-create", []solana.Instruction{ix}, false); err != nil {
			return nil, err
		}
		lookup, err = s.reader.LookupConfirmed(ctx, address)
		if err != nil {
			return nil, err
		}
	}

	if lookup == nil {
		return nil, fmt.Errorf("lookup unavailable after creation")
	}
	if lookup.State.DeactivationSlot != math.MaxUint64 {
		return nil, fmt.Errorf("upload lookup was deactivated before assembly")
	}
	if lookup.State.Authority == nil || *lookup.State.Authority != payer {
		return nil, fmt.Errorf("lookup authority mismatch")
	}

	populated := lookup.State.Addresses
	if len(populated) > len(addresses) {
		return nil, fmt.Errorf("lookup address mismatch")
	}
	for i, key := range populated {
		if key != addresses[i] {
			return nil, fmt.Errorf("lookup address mismatch")
		}
	}

	for i := len(populated); i < len(addresses); i += 20 {
		ix, err := bridge.ExtendLookup(address, payer, payer, addresses[i:min(len(addresses), i+20)])
		if err != nil {
			return nil, err
		}
		if _, err := send("lookup-extend", []solana.Instruction{ix}, false); err != nil {
			return nil, err
		}
	}

	lookup, err = s.reader.LookupConfirmed(ctx, address)
	if err != nil {
		return nil, err
	}

	if lookup == nil {
		return nil, fmt.Errorf("lookup unavailable after extension")
	}

	deadline := s.deps.now().Add(15 * time.Second)
	for {
		slot, err := s.solana.GetSlot(ctx, rpc.CommitmentConfirmed)
		if err != nil {
			return nil, err
		}
		if slot > lookup.State.LastExtendedSlot {
			return lookup, nil
		}
		if !s.deps.now().Before(deadline) {
			return nil, fmt.Errorf("lookup table did not warm up")
		}
		if err := s.deps.sleep(ctx, 100*time.Millisecond); err != nil {
			return nil, err
		}
	}
}

func (p *payerSession) cleanupHeader(ctx context.Context, f bridge.Followed, remaining []solana.PublicKey, lookup *string, send headerSender) error {
	for i := 0; i < len(remaining); i += 20 {
		ix, err := f.Cleanup(p.service.payer.PublicKey(), remaining[i:min(len(remaining), i+20)])
		if err != nil {
			return err
		}
		if _, err := send("cleanup", []solana.Instruction{ix}, false); err != nil {
			return err
		}
	}

	accounts, err := p.service.headerAccounts(ctx, remaining, rpc.CommitmentFinalized)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if account != nil && !bridge.HeaderTemporaryInitializable(account) {
			return fmt.Errorf("temporary accounts remain")
		}
	}

	if lookup == nil {
		return nil
	}

	address, err := solana.PublicKeyFromBase58(*lookup)
	if err != nil {
		return err
	}
	table, err := p.service.reader.LookupConfirmed(ctx, address)
	if err != nil {
		return err
	}
	if table != nil && table.State.DeactivationSlot == math.MaxUint64 {
		ix, err := bridge.DeactivateLookup(address, p.service.payer.PublicKey())
		if err != nil {
			return err
		}
		_, err = send("lookup-deactivate", []solana.Instruction{ix}, false)
		return err
	}
	return nil
}
