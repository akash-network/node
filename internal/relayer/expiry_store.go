package relayer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	solana "github.com/gagliardetto/solana-go"
	"golang.org/x/sys/unix"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

type expiryAssociation struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type pendingSnapshot struct {
	Path       string              `json:"path"`
	Identity   proofIdentity       `json:"identity"`
	Original   []byte              `json:"original"`
	Digest     string              `json:"digest"`
	Associated []expiryAssociation `json:"associated"`
	doc        intent
}

func (s *stateStore) expiryRoot() string { return filepath.Dir(s.ledgerPath) }

func expiryWithin(root, path string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == "." {
		return "", fmt.Errorf("expiry path outside state root")
	}
	return relative, nil
}

func expiryAncestors(root, path string) error {
	relative, err := expiryWithin(root, path)
	if err != nil {
		return err
	}
	current := root
	components := append([]string{""}, strings.Split(filepath.Dir(relative), string(filepath.Separator))...)
	for _, component := range components {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("expiry state ancestor is not a regular directory")
		}
	}
	return nil
}

func expiryRead(root, path string, limit int) ([]byte, error) {
	if err := expiryAncestors(root, path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > int64(limit) {
		return nil, fmt.Errorf("expiry evidence must be a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("expiry file read exceeded bound")
	}
	after, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(info, after) || after.Size() != int64(len(data)) || !after.ModTime().Equal(info.ModTime()) {
		return nil, fmt.Errorf("expiry file changed while reading")
	}
	return data, nil
}

func (s *stateStore) expiryPendingBinding(path string, data []byte) (pendingSnapshot, error) {
	if path != filepath.Clean(path) {
		return pendingSnapshot{}, fmt.Errorf("expiry pending path is not canonical")
	}
	relative, err := expiryWithin(s.expiryRoot(), path)
	if err != nil {
		return pendingSnapshot{}, err
	}
	identity, err := json.Marshal(s.identity)
	if err != nil {
		return pendingSnapshot{}, err
	}
	binding := append(append(append(identity, 0), []byte(relative)...), 0)
	return pendingSnapshot{
		Path:     relative,
		Identity: s.identity,
		Original: data,
		Digest:   digestHex(append(binding, data...)),
	}, nil
}

func (s *stateStore) expiryPendingCore(path string, data []byte) (pendingSnapshot, error) {
	saved, err := s.expiryPendingBinding(path, data)
	if err != nil {
		return pendingSnapshot{}, err
	}
	fields, err := jsonObject(data)
	if err != nil || requireFields(fields, documentFields(intent{})...) != nil {
		return pendingSnapshot{}, fmt.Errorf("invalid pending expiry document")
	}
	if err := json.Unmarshal(data, &saved.doc); err != nil {
		return pendingSnapshot{}, err
	}
	if err := s.validateIntent(&saved.doc); err != nil {
		return pendingSnapshot{}, err
	}
	if _, err := validateOrdinaryIntent(&saved.doc); err != nil {
		return pendingSnapshot{}, err
	}
	return saved, nil
}

func (s *stateStore) snapshotPending(path string) (pendingSnapshot, error) {
	root := s.expiryRoot()
	data, err := expiryRead(root, path, 16<<10)
	if err != nil {
		return pendingSnapshot{}, err
	}
	saved, err := s.expiryPendingCore(path, data)
	if err != nil {
		return pendingSnapshot{}, err
	}
	doc := saved.doc
	associate := func(associated string) ([]byte, error) {
		data, err := expiryRead(root, associated, 2<<20)
		if err != nil {
			return nil, err
		}
		path, err := expiryWithin(root, associated)
		if err != nil {
			return nil, err
		}
		saved.Associated = append(saved.Associated, expiryAssociation{
			Path:   path,
			Digest: digestHex(data),
		})
		return data, nil
	}
	absolute, _ := filepath.Abs(path)
	jobs, _ := filepath.Abs(s.jobDirectory)
	headers, _ := filepath.Abs(s.headerDirectory)
	switch filepath.Dir(absolute) {
	case jobs:
		switch doc.Stage {
		case "proof-create", "proof-grow", "proof-append", "proof-close", "resolve":
		default:
			return pendingSnapshot{}, fmt.Errorf("expiry stage does not belong to payment path")
		}
		proof, err := associate(path + ".proof")
		if err != nil {
			return pendingSnapshot{}, err
		}
		var identity struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(proof, &identity); err != nil {
			return pendingSnapshot{}, err
		}
		id, err := native.ParseID(identity.ID)
		if err != nil {
			return pendingSnapshot{}, err
		}
		if _, err := parseProofJob(proof, s.identity, id); err != nil {
			return pendingSnapshot{}, err
		}
		expected, err := s.paymentPath(id)
		if err != nil {
			return pendingSnapshot{}, err
		}
		expected, _ = filepath.Abs(expected)
		if absolute != expected {
			return pendingSnapshot{}, fmt.Errorf("expiry payment filename mismatch")
		}
	case headers:
		name := filepath.Base(path)
		suffix := ".pending.json"
		if strings.HasSuffix(name, ".close.pending.json") {
			suffix = ".close.pending.json"
			if doc.Stage != "lookup-close" {
				return pendingSnapshot{}, fmt.Errorf("expiry close path stage mismatch")
			}
		} else {
			switch doc.Stage {
			case "signature-batch-create", "signature", "chunk", "assembly", "cleanup", "lookup-create", "lookup-extend", "lookup-deactivate":
			default:
				return pendingSnapshot{}, fmt.Errorf("expiry session path stage mismatch")
			}
		}
		id := strings.TrimSuffix(name, suffix)
		expected, err := s.sessionPath(id, suffix)
		if err != nil {
			return pendingSnapshot{}, err
		}
		expected, _ = filepath.Abs(expected)
		if expected != absolute {
			return pendingSnapshot{}, fmt.Errorf("expiry session filename mismatch")
		}
		sessionPath, _ := s.sessionPath(id, ".json")
		data, err := associate(sessionPath)
		if err != nil {
			return pendingSnapshot{}, err
		}
		session, err := expirySession(data)
		if err != nil {
			return pendingSnapshot{}, err
		}
		if err := s.validateSession(id, &session); err != nil {
			return pendingSnapshot{}, err
		}
		if !expiryHash(session.Client) {
			return pendingSnapshot{}, fmt.Errorf("expiry session client identity invalid")
		}
		if doc.Stage == "lookup-close" && session.Lookup == nil {
			return pendingSnapshot{}, fmt.Errorf("expiry close has no associated lookup")
		}
	default:
		return pendingSnapshot{}, fmt.Errorf("expiry pending path is not deployment-owned")
	}
	return saved, nil
}

// Legacy documents preserve extension fields, but case aliases of evidence
// fields can hide terminal observations in the case-insensitive Go decoder.
func expiryEvidenceAliases(fields map[string]json.RawMessage, names ...string) error {
	for field := range fields {
		for _, name := range names {
			if field != name && strings.EqualFold(field, name) {
				return fmt.Errorf("ambiguous expiry evidence field %s", name)
			}
		}
	}
	return nil
}

func expiryEvidenceSignature(fields map[string]json.RawMessage, name string) (string, error) {
	var value string
	if err := historyField(fields, name, &value); err != nil {
		return "", err
	}
	signature, err := solana.SignatureFromBase58(value)
	if err != nil || signature.String() != value {
		return "", fmt.Errorf("invalid expiry evidence signature %s", name)
	}
	return value, nil
}

func expirySession(data []byte) (sessionDoc, error) {
	fields, err := jsonObject(data)
	if err != nil {
		return sessionDoc{}, err
	}
	if err := expiryEvidenceAliases(fields, "assembly", "lookupCloseFailure", "lookupCloseTransaction"); err != nil {
		return sessionDoc{}, err
	}
	for _, name := range []string{"assembly", "lookupCloseFailure"} {
		raw, exists := fields[name]
		if !exists {
			continue
		}
		transaction, err := jsonObject(raw)
		if err != nil {
			return sessionDoc{}, err
		}
		if err := expiryEvidenceAliases(transaction, "signature"); err != nil {
			return sessionDoc{}, err
		}
		if _, err := expiryEvidenceSignature(transaction, "signature"); err != nil {
			return sessionDoc{}, err
		}
	}
	if _, exists := fields["lookupCloseTransaction"]; exists {
		if _, err := expiryEvidenceSignature(fields, "lookupCloseTransaction"); err != nil {
			return sessionDoc{}, err
		}
	}
	var session sessionDoc
	if err := json.Unmarshal(data, &session); err != nil {
		return sessionDoc{}, err
	}
	return session, nil
}

// Reads stop at the original size. The payer lock excludes cooperating writers;
// these checks also refuse changes observed while the stream is being consumed.
func expiryStream(ctx context.Context, root, path string, visit func(io.Reader) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := expiryAncestors(root, path); err != nil {
		return err
	}
	before, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() {
		return fmt.Errorf("expiry stream must be a regular file")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	if !expirySameFile(before, opened) {
		return fmt.Errorf("expiry file changed before streaming")
	}
	reader := &expiryStreamReader{
		ctx:    ctx,
		reader: io.LimitReader(file, opened.Size()),
	}
	if err := visit(reader); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	after, err := file.Stat()
	if err != nil {
		return err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !expirySameFile(opened, after) || !expirySameFile(opened, pathInfo) || reader.read != opened.Size() {
		return fmt.Errorf("expiry file changed while streaming")
	}
	return expiryAncestors(root, path)
}

type expiryStreamReader struct {
	ctx    context.Context
	reader io.Reader
	read   int64
}

func (r *expiryStreamReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(data)
	r.read += int64(n)
	return n, err
}

func expirySameFile(before, after os.FileInfo) bool {
	return before.Mode() == after.Mode() && os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}

// Submission records are not observations. Terminal session, ledger or header
// journal evidence contradicts absence even if the provider has lost history.
func (s *stateStore) expiryContradictions(ctx context.Context, saved pendingSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var data []byte
	_, err := os.Lstat(s.ledgerPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		err = expiryStream(ctx, s.expiryRoot(), s.ledgerPath, func(reader io.Reader) error {
			var readErr error
			data, readErr = io.ReadAll(reader)
			return readErr
		})
		if err != nil {
			return fmt.Errorf("expiry ledger evidence: %w", err)
		}
	}
	ledger, err := parseSpendLedger(data, s.identity.Genesis, s.identity.ProgramID)
	if err != nil {
		return err
	}
	for _, entry := range ledger.entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, signature := range entry.Signatures {
			if err := ctx.Err(); err != nil {
				return err
			}
			if signature == saved.doc.Signature {
				return fmt.Errorf("ledger observation contradicts expiry non-inclusion")
			}
		}
	}
	if err := s.expirySessionContradictions(ctx, saved); err != nil {
		return err
	}
	path := filepath.Join(s.headerDirectory, "transactions.jsonl")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return expiryStream(ctx, s.expiryRoot(), path, func(reader io.Reader) error {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 4096), expiryMaxResponse)
		line := 0
		for scanner.Scan() {
			line++
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := expiryJournalContradiction(scanner.Bytes(), saved.doc.Signature); err != nil {
				return fmt.Errorf("expiry header journal line %d: %w; preserve evidence and repair from verified history before retrying", line, err)
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("expiry header journal line %d unreadable (2 MiB line limit): %w; preserve evidence and repair from verified history before retrying", line+1, err)
		}
		return nil
	})
}

func (s *stateStore) expirySessionContradictions(ctx context.Context, saved pendingSnapshot) error {
	path := filepath.Join(s.headerDirectory, "transactions.jsonl")
	if err := expiryAncestors(s.expiryRoot(), path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	before, err := os.Lstat(s.headerDirectory)
	if err != nil {
		return err
	}
	directory, err := os.OpenFile(s.headerDirectory, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer directory.Close()
	opened, err := directory.Stat()
	if err != nil {
		return err
	}
	if !opened.IsDir() || !expirySameFile(before, opened) {
		return fmt.Errorf("expiry session directory changed before reading")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := directory.ReadDir(128)
		if err != nil && err != io.EOF {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			id := strings.TrimSuffix(entry.Name(), ".json")
			if !strings.HasSuffix(entry.Name(), ".json") || !validHex(id, 32, true) {
				continue
			}
			data, err := expiryRead(s.expiryRoot(), filepath.Join(s.headerDirectory, entry.Name()), 2<<20)
			if err != nil {
				return fmt.Errorf("expiry session observation unavailable: %w", err)
			}
			session, err := expirySession(data)
			if err != nil || session.Session != id || session.Genesis != s.identity.Genesis {
				return fmt.Errorf("invalid expiry session observation")
			}
			if (session.Assembly != nil && session.Assembly.Signature == saved.doc.Signature) ||
				(session.LookupCloseTransaction != nil && *session.LookupCloseTransaction == saved.doc.Signature) ||
				(session.LookupCloseFailure != nil && session.LookupCloseFailure.Signature == saved.doc.Signature) {
				return fmt.Errorf("session observation contradicts expiry non-inclusion")
			}
		}
		if err == io.EOF {
			break
		}
	}
	after, err := directory.Stat()
	if err != nil {
		return err
	}
	pathInfo, err := os.Lstat(s.headerDirectory)
	if err != nil {
		return err
	}
	if !expirySameFile(opened, after) || !expirySameFile(opened, pathInfo) {
		return fmt.Errorf("expiry session directory changed while reading")
	}
	return expiryAncestors(s.expiryRoot(), path)
}

func expiryJournalContradiction(data []byte, pendingSignature string) error {
	fields, err := jsonObject(data)
	if err != nil {
		return fmt.Errorf("invalid expiry header journal observation")
	}
	var event string
	if historyField(fields, "event", &event) != nil || event == "" {
		return fmt.Errorf("invalid expiry header journal event")
	}
	required := ""
	switch event {
	case "transaction", "submitted":
		required = "signature"
	case "authenticated-concurrently":
		required = "localAssembly"
	}
	for _, name := range []string{"signature", "localAssembly"} {
		_, exists := fields[name]
		if !exists && name != required {
			continue
		}
		signature, err := expiryEvidenceSignature(fields, name)
		if err != nil {
			return err
		}
		if signature == pendingSignature && !(event == "submitted" && name == "signature") {
			return fmt.Errorf("header journal observation contradicts expiry non-inclusion")
		}
	}
	return nil
}

type expiryCapsule struct {
	Version         int                      `json:"version"`
	Pending         pendingSnapshot          `json:"pending"`
	Certificate     *nonInclusionCertificate `json:"certificate"`
	CertificateHash string                   `json:"certificateHash"`
}

func (s *stateStore) expiryCapsulePath(saved pendingSnapshot) string {
	return filepath.Join(s.expiryRoot(), "expired-intents-v1", s.identity.Genesis, s.identity.ProgramID, s.identity.Payer, saved.Digest+".json")
}

func expirySamePending(a, b pendingSnapshot) bool {
	return a.Path == b.Path && a.Identity == b.Identity && a.Digest == b.Digest && bytes.Equal(a.Original, b.Original)
}

func (s *stateStore) expiryGenerationPath(saved pendingSnapshot, endpoint string) string {
	return strings.TrimSuffix(s.expiryCapsulePath(saved), ".json") + ".endpoint-" + endpoint + ".json"
}

// This reads historical evidence without requiring the old association to equal
// today's document. snapshotPending validates today's association separately.
func (s *stateStore) readExpiryCapsule(saved pendingSnapshot, path string) (*expiryCapsule, error) {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	data, err := expiryRead(s.expiryRoot(), path, 80<<20)
	if err != nil {
		return nil, err
	}
	var capsule expiryCapsule
	if err := json.Unmarshal(data, &capsule); err != nil {
		return nil, fmt.Errorf("invalid expiry capsule JSON")
	}
	canonical, err := json.Marshal(capsule)
	if err != nil || !bytes.Equal(append(canonical, '\n'), data) {
		return nil, fmt.Errorf("noncanonical or incomplete expiry capsule")
	}
	if capsule.Version != 1 || !expirySamePending(capsule.Pending, saved) {
		return nil, fmt.Errorf("immutable expiry capsule identity collision")
	}
	associated := saved.Path + ".proof"
	if strings.HasSuffix(saved.Path, ".pending.json") {
		suffix := ".pending.json"
		if saved.doc.Stage == "lookup-close" {
			suffix = ".close.pending.json"
		}
		associated = strings.TrimSuffix(saved.Path, suffix) + ".json"
	}
	if len(capsule.Pending.Associated) != 1 || capsule.Pending.Associated[0].Path != associated || !validHex(capsule.Pending.Associated[0].Digest, 32, true) {
		return nil, fmt.Errorf("invalid expiry capsule association evidence")
	}
	certificate, err := json.Marshal(capsule.Certificate)
	if err != nil || digestHex(certificate) != capsule.CertificateHash {
		return nil, fmt.Errorf("expiry capsule certificate digest mismatch")
	}
	if err := validateExpiryCertificate(saved, capsule.Certificate); err != nil {
		return nil, err
	}
	return &capsule, nil
}

// The legacy primary remains available for evidence inspection. Recovery uses
// loadExpiryCertificateFor so another provider never inherits cached evidence.
func (s *stateStore) loadExpiryCertificate(saved pendingSnapshot) (*nonInclusionCertificate, error) {
	if err := s.expiryNotSuperseded(saved); err != nil {
		return nil, err
	}
	capsule, err := s.readExpiryCapsule(saved, s.expiryCapsulePath(saved))
	if err != nil || capsule == nil {
		return nil, err
	}
	return capsule.Certificate, nil
}

func (s *stateStore) expiryCertificateLocation(saved pendingSnapshot, endpoint string) (string, *nonInclusionCertificate, error) {
	if !validHex(endpoint, 32, true) {
		return "", nil, fmt.Errorf("invalid expiry endpoint identity")
	}
	if err := s.expiryNotSuperseded(saved); err != nil {
		return "", nil, err
	}
	path := s.expiryCapsulePath(saved)
	primary, err := s.readExpiryCapsule(saved, path)
	if err != nil {
		return "", nil, err
	}
	if primary == nil {
		return path, nil, nil
	}
	if primary.Certificate.Endpoint == endpoint {
		return path, primary.Certificate, nil
	}
	path = s.expiryGenerationPath(saved, endpoint)
	generation, err := s.readExpiryCapsule(saved, path)
	if err != nil || generation == nil {
		return path, nil, err
	}
	if generation.Certificate.Endpoint != endpoint {
		return "", nil, fmt.Errorf("immutable expiry generation endpoint collision")
	}
	return path, generation.Certificate, nil
}

func (s *stateStore) loadExpiryCertificateFor(saved pendingSnapshot, endpoint string) (*nonInclusionCertificate, error) {
	_, certificate, err := s.expiryCertificateLocation(saved, endpoint)
	return certificate, err
}

type expirySupersession struct {
	Version         int             `json:"version"`
	Pending         pendingSnapshot `json:"pending"`
	CertificateHash string          `json:"certificateHash"`
	Receipt         *transactionDoc `json:"receipt"`
}

func (s *stateStore) expirySupersessionPath(saved pendingSnapshot) string {
	return strings.TrimSuffix(s.expiryCapsulePath(saved), ".json") + ".superseded.json"
}

func (s *stateStore) readExpirySupersession(saved pendingSnapshot) (*expirySupersession, error) {
	path := s.expirySupersessionPath(saved)
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	data, err := expiryRead(s.expiryRoot(), path, 80<<20)
	if err != nil {
		return nil, err
	}
	var marker expirySupersession
	if err := json.Unmarshal(data, &marker); err != nil {
		return nil, fmt.Errorf("invalid expiry supersession marker JSON")
	}
	canonical, err := json.Marshal(marker)
	if err != nil || !bytes.Equal(append(canonical, '\n'), data) {
		return nil, fmt.Errorf("noncanonical or incomplete expiry supersession marker")
	}
	if marker.Version != 1 || !expirySamePending(marker.Pending, saved) || len(marker.Pending.Associated) != 0 ||
		!validHex(marker.CertificateHash, 32, true) || marker.Receipt == nil ||
		marker.Receipt.Signature != saved.doc.Signature || marker.Receipt.Stage != saved.doc.Stage {
		return nil, fmt.Errorf("immutable expiry supersession identity collision")
	}
	return &marker, nil
}

func (s *stateStore) expiryNotSuperseded(saved pendingSnapshot) error {
	marker, err := s.readExpirySupersession(saved)
	if err != nil {
		return err
	}
	if marker != nil {
		return fmt.Errorf("expiry certificate superseded by observed transaction; absence evidence cannot be reused")
	}
	return nil
}

func (p *payerSession) reconcileExpiryReceipt(path string, doc *intent, result txResult) error {
	return p.reconcileExpiryReceiptWith(path, doc, result, expiryFiles())
}

func (p *payerSession) reconcileExpiryReceiptWith(path string, doc *intent, result txResult, ops expiryFileOps) error {
	s := p.service.store
	// Preserve the ordinary receipt path when no absence evidence exists. Only
	// that path's existing pending-file read is needed for the namespace lookup.
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	candidate, err := s.expiryPendingBinding(path, data)
	if err != nil {
		return err
	}
	exists := false
	for _, evidence := range []string{s.expiryCapsulePath(candidate), s.expirySupersessionPath(candidate)} {
		if _, err := os.Lstat(evidence); err == nil {
			exists = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if !exists {
		return nil
	}
	data, err = expiryRead(s.expiryRoot(), path, 16<<10)
	if err != nil {
		return err
	}
	saved, err := s.expiryPendingCore(path, data)
	if err != nil {
		return err
	}
	if !expirySamePending(candidate, saved) || saved.doc != *doc || result.signature != doc.Signature || result.stage != doc.Stage {
		return fmt.Errorf("pending intent changed before expiry receipt reconciliation")
	}
	primary, err := s.readExpiryCapsule(saved, s.expiryCapsulePath(saved))
	if err != nil {
		return err
	}
	if primary == nil {
		return fmt.Errorf("expiry supersession has no original capsule")
	}
	marker := expirySupersession{
		Version:         1,
		Pending:         saved,
		CertificateHash: primary.CertificateHash,
		Receipt:         transactionDocument(result),
	}
	markerPath := s.expirySupersessionPath(saved)
	prior, err := s.readExpirySupersession(saved)
	if err != nil {
		return err
	}
	if prior != nil {
		priorBytes, priorErr := json.Marshal(prior)
		markerBytes, markerErr := json.Marshal(marker)
		if priorErr != nil || markerErr != nil || !bytes.Equal(priorBytes, markerBytes) {
			return fmt.Errorf("immutable expiry supersession receipt collision")
		}
		// A previous attempt may have renamed successfully but failed its sync.
		if err := syncExpiryDocument(markerPath, ops); err != nil {
			return err
		}
	} else {
		encoded, err := json.Marshal(marker)
		if err != nil {
			return err
		}
		if err := ops.directories(s.expiryRoot(), markerPath); err != nil {
			return err
		}
		if err := publishExpiryDocument(markerPath, encoded, ops); err != nil {
			return err
		}
	}
	p.service.emit(true, map[string]any{
		"event":         "expiry-certificate-superseded",
		"stage":         doc.Stage,
		"signature":     doc.Signature,
		"pendingDigest": saved.Digest,
		"certificate":   primary.CertificateHash,
		"reason":        "observed transaction supersedes certified absence; investigate provider history completeness and finality",
	})
	return nil
}

func syncExpiryDocument(path string, ops expiryFileOps) error {
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := ops.sync(file); err != nil {
		return err
	}
	return ops.syncDirectory(filepath.Dir(path))
}

// Injectable filesystem operations exercise actual publication failure points
// without changing the legacy writer or installing process-global test hooks.
type expiryFileOps struct {
	write  func(*os.File, []byte) (int, error)
	sync   func(*os.File) error
	rename func(string, string) error
	unlink func(string) error
}

func expiryFiles() expiryFileOps {
	return expiryFileOps{
		write:  (*os.File).Write,
		sync:   (*os.File).Sync,
		rename: os.Rename,
		unlink: os.Remove,
	}
}

func (ops expiryFileOps) syncDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return ops.sync(file)
}

func (ops expiryFileOps) directories(root, destination string) error {
	relative, err := expiryWithin(root, destination)
	if err != nil {
		return err
	}
	current := root
	for _, component := range strings.Split(filepath.Dir(relative), string(filepath.Separator)) {
		next := filepath.Join(current, component)
		if err := os.Mkdir(next, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := os.Lstat(next)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("invalid expiry capsule ancestor")
		}
		// Sync existing ancestors too: a prior crash may have occurred between
		// mkdir and publication of that directory in its own parent.
		if err := ops.syncDirectory(current); err != nil {
			return err
		}
		current = next
	}
	return ops.syncDirectory(current)
}

func (s *stateStore) publishExpiry(saved pendingSnapshot, cert *nonInclusionCertificate, ops expiryFileOps) error {
	if err := validateExpiryCertificate(saved, cert); err != nil {
		return err
	}
	path, cached, err := s.expiryCertificateLocation(saved, cert.Endpoint)
	if err != nil {
		return err
	}
	if path != s.expiryCapsulePath(saved) {
		primary, err := s.readExpiryCapsule(saved, s.expiryCapsulePath(saved))
		if err != nil {
			return err
		}
		if primary == nil {
			return fmt.Errorf("expiry generation has no original capsule")
		}
		if err := validateExpiryBlockViews(append(primary.Certificate.blockViews(), cert.blockViews()...)); err != nil {
			return fmt.Errorf("expiry provider generation contradicts original evidence: %w", err)
		}
	}
	if err := ops.directories(s.expiryRoot(), path); err != nil {
		return err
	}
	if cached != nil {
		if !reflect.DeepEqual(cached, cert) {
			return fmt.Errorf("immutable expiry certificate collision")
		}
		return syncExpiryDocument(path, ops)
	}
	certificate, err := json.Marshal(cert)
	if err != nil {
		return err
	}
	data, err := json.Marshal(expiryCapsule{
		Version:         1,
		Pending:         saved,
		Certificate:     cert,
		CertificateHash: digestHex(certificate),
	})
	if err != nil {
		return err
	}
	return publishExpiryDocument(path, data, ops)
}

func publishExpiryDocument(path string, data []byte, ops expiryFileOps) error {
	data = append(data, '\n')
	file, err := os.CreateTemp(filepath.Dir(path), ".expiry-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	count, err := ops.write(file, data)
	if err == nil && count != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = ops.sync(file)
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("expiry evidence appeared during publication")
	}
	if err := ops.rename(temporary, path); err != nil {
		return err
	}
	return ops.syncDirectory(filepath.Dir(path))
}

func (p *payerSession) retireExpired(ctx context.Context, saved pendingSnapshot, cert *nonInclusionCertificate) error {
	return p.retireExpiredWith(ctx, saved, cert, expiryFiles())
}

func (p *payerSession) retireExpiredWith(ctx context.Context, saved pendingSnapshot, cert *nonInclusionCertificate, ops expiryFileOps) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateExpiryCertificate(saved, cert); err != nil {
		return err
	}
	s := p.service.store
	if err := s.publishExpiry(saved, cert, ops); err != nil {
		return err
	}
	if err := s.expiryContradictions(ctx, saved); err != nil {
		return err
	}
	if err := s.expiryNotSuperseded(saved); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path := filepath.Join(s.expiryRoot(), saved.Path)
	current, err := s.snapshotPending(path)
	if err != nil {
		return fmt.Errorf("recheck pending expiry snapshot: %w", err)
	}
	if !reflect.DeepEqual(current, saved) {
		return fmt.Errorf("pending bytes or associated identity changed before expiry retirement")
	}
	// snapshotPending also rereads associations. Make the final file operation
	// before unlink a bounded regular-file comparison of the exact pending bytes.
	data, err := expiryRead(s.expiryRoot(), path, 16<<10)
	if err != nil {
		return fmt.Errorf("read pending immediately before expiry unlink: %w", err)
	}
	if !bytes.Equal(data, saved.Original) {
		return fmt.Errorf("pending bytes changed immediately before expiry unlink")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ops.unlink(path); err != nil {
		return err
	}
	return ops.syncDirectory(filepath.Dir(path))
}
