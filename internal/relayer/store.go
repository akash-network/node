package relayer

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	solana "github.com/gagliardetto/solana-go"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/native"
	"pkg.akt.dev/node/v2/x/sink/keeper"
)

type stateStore struct {
	identity        proofIdentity
	writeJSON       func(string, []byte) error
	headerDirectory string
	payerLock       string
	ledgerPath      string
	ledgerLock      string
	jobDirectory    string
	discoveryPath   string
	activePath      string
}

func newStateStore(root, chainID, genesis, program, payer string, writeJSON func(string, []byte) error) (*stateStore, error) {
	if root == "" || chainID == "" {
		return nil, fmt.Errorf("state root and chain ID are required")
	}
	for _, value := range []string{genesis, program, payer} {
		if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
			return nil, fmt.Errorf("invalid state deployment identity")
		}
	}
	if writeJSON == nil {
		writeJSON = atomicJSON
	}
	directory := filepath.Join(root, "header-uploads-v2", genesis, program)
	return &stateStore{
		identity: proofIdentity{
			ChainID:   chainID,
			Genesis:   genesis,
			ProgramID: program,
			Payer:     payer,
		},
		writeJSON:       writeJSON,
		headerDirectory: directory,
		payerLock:       filepath.Join(directory, "payer.lock"),
		ledgerPath:      filepath.Join(root, "relay-budget-v2-"+genesis+"-"+program+".json"),
		ledgerLock:      filepath.Join(root, "relay-budget.lock"),
		jobDirectory:    filepath.Join(root, "relay-jobs-v2", genesis, program),
		discoveryPath:   filepath.Join(root, "relay-discovery-v3-"+genesis+"-"+program+".json"),
		activePath:      filepath.Join(directory, "active.json"),
	}, nil
}

func (s *stateStore) saveCheckpoint(height int64, blockHash, root string) error {
	return s.save(filepath.Join(filepath.Dir(s.ledgerPath), "checkpoint.json"), struct {
		ChainID   string `json:"chainId"`
		Height    int64  `json:"height"`
		BlockHash string `json:"blockHash"`
		Root      string `json:"root"`
	}{
		ChainID:   s.identity.ChainID,
		Height:    height,
		BlockHash: blockHash,
		Root:      root,
	})
}

// The temporary file must be adjacent to the destination for atomic rename,
// including deployed state outside the repository's scratch directory.
func atomicJSON(path string, data []byte) (err error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	parent, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

func optionalJSON(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("invalid JSON in %s", path)
	}
	return data, nil
}

func (s *stateStore) save(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.writeJSON(path, append(data, '\n'))
}

func (s *stateStore) sessionPath(id, suffix string) (string, error) {
	if !validHex(id, 32, true) {
		return "", fmt.Errorf("invalid header session ID")
	}
	return filepath.Join(s.headerDirectory, id+suffix), nil
}

func (s *stateStore) sessionPendingPath(id string) (string, error) {
	return s.sessionPath(id, ".pending.json")
}

func (s *stateStore) sessionClosePendingPath(id string) (string, error) {
	return s.sessionPath(id, ".close.pending.json")
}

func (s *stateStore) validateSession(id string, doc *sessionDoc) error {
	if doc == nil || doc.Version != 1 || doc.Genesis != s.identity.Genesis || doc.Payer != s.identity.Payer || doc.Session != id {
		return fmt.Errorf("header session belongs to another deployment or session")
	}
	if doc.Client == "" || !positiveSafe(doc.Target) || !positiveSafe(doc.Trusted) || doc.Trusted >= doc.Target {
		return fmt.Errorf("invalid header session identity")
	}
	if doc.Lookup != nil {
		if _, err := solana.PublicKeyFromBase58(*doc.Lookup); err != nil {
			return err
		}
		if doc.LookupSlot == nil || !safeInteger(*doc.LookupSlot) {
			return fmt.Errorf("invalid header lookup slot")
		}
	}
	for _, amount := range []*int64{doc.LookupRentClosing, doc.LookupRentReturned} {
		if amount != nil && (*amount < 0 || !safeInteger(*amount)) {
			return fmt.Errorf("invalid lookup rent")
		}
	}
	if doc.Assembly != nil && doc.Assembly.Stage != "assembly" {
		return fmt.Errorf("invalid terminal assembly stage")
	}
	return nil
}

func (s *stateStore) loadSession(id string) (*sessionDoc, error) {
	path, err := s.sessionPath(id, ".json")
	if err != nil {
		return nil, err
	}
	data, err := optionalJSON(path)
	if err != nil || data == nil {
		return nil, err
	}
	var doc sessionDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if err := s.validateSession(id, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *stateStore) saveSession(id string, doc *sessionDoc) error {
	path, err := s.sessionPath(id, ".json")
	if err != nil {
		return err
	}
	if err := s.validateSession(id, doc); err != nil {
		return err
	}
	return s.save(path, doc)
}

func (s *stateStore) sessionIDs() ([]string, error) {
	files, err := os.ReadDir(s.headerDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, file := range files {
		name := file.Name()
		if strings.HasSuffix(name, ".json") && validHex(strings.TrimSuffix(name, ".json"), 32, true) {
			ids = append(ids, strings.TrimSuffix(name, ".json"))
		}
	}
	return ids, nil
}

func (s *stateStore) validateActive(doc *activeHeaderDoc) error {
	if doc == nil || doc.ChainID != s.identity.ChainID || doc.Genesis != s.identity.Genesis {
		return fmt.Errorf("active header belongs to another deployment")
	}
	if !positiveSafe(doc.Target) || !positiveSafe(doc.Trusted) || doc.Trusted >= doc.Target {
		return fmt.Errorf("invalid active header heights")
	}
	return nil
}

func (s *stateStore) loadActive() (*activeHeaderDoc, error) {
	data, err := optionalJSON(s.activePath)
	if err != nil || data == nil {
		return nil, err
	}
	var doc activeHeaderDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if err := s.validateActive(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *stateStore) saveActive(doc *activeHeaderDoc) error {
	if err := s.validateActive(doc); err != nil {
		return err
	}
	if _, err := decodeActivePacket(doc.Packet, s.identity.ChainID); err != nil {
		return err
	}
	return s.save(s.activePath, doc)
}

func (s *stateStore) abandonActive(height headerHeight, now time.Time) (string, error) {
	path := filepath.Join(s.headerDirectory, fmt.Sprintf("abandoned-%d-%d.json", height, now.UnixMilli()))
	if err := os.Rename(s.activePath, path); err != nil {
		return "", err
	}
	return path, nil
}

func (s *stateStore) removeActive() error { return os.Remove(s.activePath) }

func (s *stateStore) appendHeaderEvent(event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.headerDirectory, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(s.headerDirectory, "transactions.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (s *stateStore) paymentPath(id sink.ID) (string, error) {
	if _, err := native.ParseID(id.String()); err != nil {
		return "", err
	}
	key, err := keeper.TransferKey(id)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(key)
	return filepath.Join(s.jobDirectory, hex.EncodeToString(hash[:])+".json"), nil
}

func (s *stateStore) proofPath(id sink.ID) (string, error) {
	path, err := s.paymentPath(id)
	if err != nil {
		return "", err
	}
	return path + ".proof", nil
}

func (s *stateStore) loadProof(id sink.ID) (*proofJob, error) {
	path, err := s.proofPath(id)
	if err != nil {
		return nil, err
	}
	data, err := optionalJSON(path)
	if err != nil || data == nil {
		return nil, err
	}
	return parseProofJob(data, s.identity, id)
}

func (s *stateStore) saveProof(doc *proofJob) error {
	if doc == nil {
		return fmt.Errorf("missing proof job")
	}
	id, err := native.ParseID(doc.ID)
	if err != nil {
		return err
	}
	if err := doc.validate(s.identity, id); err != nil {
		return err
	}
	path, err := s.proofPath(id)
	if err != nil {
		return err
	}
	return s.save(path, doc)
}

func (s *stateStore) removeProof(id sink.ID) error {
	path, err := s.proofPath(id)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (s *stateStore) proofJobIDs() ([]sink.ID, error) {
	files, err := os.ReadDir(s.jobDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return []sink.ID{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := []sink.ID{}
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json.proof") {
			continue
		}
		data, err := optionalJSON(filepath.Join(s.jobDirectory, file.Name()))
		if err != nil {
			return nil, err
		}
		if data == nil {
			continue
		}
		var doc proofJob
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		id, err := native.ParseID(doc.ID)
		if err != nil {
			return nil, err
		}
		if _, err := parseProofJob(data, s.identity, id); err != nil {
			return nil, err
		}
		path, err := s.proofPath(id)
		if err != nil {
			return nil, err
		}
		if filepath.Base(path) != file.Name() {
			return nil, fmt.Errorf("proof journal filename mismatch")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

type discoveryDoc struct {
	Version   int                 `json:"version"`
	ChainID   string              `json:"chainId"`
	Genesis   string              `json:"genesis"`
	ProgramID string              `json:"programId"`
	Cursor    *discoveryCursorDoc `json:"cursor"`
}

type discoveryCursorDoc struct {
	Key    string `json:"key"`
	LastID string `json:"lastId"`
}

func (s *stateStore) loadDiscovery() (*native.Cursor, error) {
	data, err := optionalJSON(s.discoveryPath)
	if err != nil || data == nil {
		return nil, err
	}
	fields, err := jsonObject(data)
	if err != nil {
		return nil, err
	}
	if _, ok := fields["cursor"]; !ok {
		return nil, fmt.Errorf("missing discovery cursor")
	}
	var doc discoveryDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Version != 3 || doc.ChainID != s.identity.ChainID || doc.Genesis != s.identity.Genesis || doc.ProgramID != s.identity.ProgramID {
		return nil, fmt.Errorf("unsupported discovery state; preserve it and use a fresh discovery file")
	}
	if doc.Cursor == nil {
		return nil, nil
	}
	cursorFields, err := jsonObject(fields["cursor"])
	if err != nil {
		return nil, err
	}
	if len(cursorFields) != 2 {
		return nil, fmt.Errorf("invalid discovery cursor fields")
	}
	if err := requireFields(cursorFields, "key", "lastId"); err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(doc.Cursor.Key)
	if err != nil || base64.StdEncoding.EncodeToString(key) != doc.Cursor.Key {
		return nil, fmt.Errorf("invalid discovery cursor base64")
	}
	id, err := native.ParseID(doc.Cursor.LastID)
	if err != nil {
		return nil, err
	}
	return &native.Cursor{
		Kind:   "tail",
		Key:    key,
		LastID: id,
	}, nil
}

func (s *stateStore) saveDiscovery(cursor *native.Cursor) error {
	doc := discoveryDoc{
		Version:   3,
		ChainID:   s.identity.ChainID,
		Genesis:   s.identity.Genesis,
		ProgramID: s.identity.ProgramID,
	}
	if cursor != nil {
		if cursor.Kind != "tail" {
			return fmt.Errorf("only a completed discovery tail can be saved")
		}
		if _, err := native.ParseID(cursor.LastID.String()); err != nil {
			return err
		}
		doc.Cursor = &discoveryCursorDoc{
			Key:    base64.StdEncoding.EncodeToString(cursor.Key),
			LastID: cursor.LastID.String(),
		}
	}
	return s.save(s.discoveryPath, doc)
}

func (s *stateStore) readLedger() (*spendLedger, error) {
	data, err := optionalJSON(s.ledgerPath)
	if err != nil {
		return nil, err
	}
	return parseSpendLedger(data, s.identity.Genesis, s.identity.ProgramID)
}

func (s *stateStore) updateLedger(ctx context.Context, change func(*spendLedger) (bool, error)) (*spendLedger, error) {
	var ledger *spendLedger
	acquired, err := withFileLock(ctx, s.ledgerLock, 10*time.Second, func() error {
		var err error
		ledger, err = s.readLedger()
		if err != nil {
			return err
		}
		changed, err := change(ledger)
		if err != nil || !changed {
			return err
		}
		return s.save(s.ledgerPath, ledger.document(s.identity.Genesis, s.identity.ProgramID))
	})
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, fmt.Errorf("relay budget lock unavailable")
	}
	return ledger, nil
}
