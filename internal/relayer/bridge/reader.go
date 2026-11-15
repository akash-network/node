package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"

	sink "pkg.akt.dev/go/node/sink/v1"
)

// FollowMismatch preserves discovery details for the core's structured alerts.
// Found contains the bridge's Program, ChainID and Slot; discovery stopped
// before reading the mismatched client's app or state.
type FollowMismatch struct {
	Found   Followed
	Event   string
	Message string
}

func (e *FollowMismatch) Error() string { return e.Message }

type Reader struct {
	rpc     *rpc.Client
	program solana.PublicKey
	chainID string
	pin     *solana.PublicKey
}

func NewReader(client *rpc.Client, program solana.PublicKey, chainID string, pin *solana.PublicKey) *Reader {
	var pinned *solana.PublicKey
	if pin != nil {
		value := *pin
		pinned = &value
	}
	return &Reader{
		rpc:     client,
		program: program,
		chainID: chainID,
		pin:     pinned,
	}
}

// accountResponse preserves the difference between an absent account and an
// incomplete RPC response. solana-go collapses a missing value into ErrNotFound.
func (r *Reader) accountResponse(ctx context.Context, method string, key any, commitment rpc.CommitmentType, minSlot *uint64) (json.RawMessage, uint64, error) {
	options := rpc.M{
		"encoding":   solana.EncodingBase64,
		"commitment": commitment,
	}
	if minSlot != nil {
		options["minContextSlot"] = *minSlot
	}
	var raw json.RawMessage
	if err := r.rpc.RPCCallForInto(ctx, &raw, method, []interface{}{key, options}); err != nil {
		return nil, 0, err
	}
	var response struct {
		Context *struct {
			Slot *uint64 `json:"slot"`
		} `json:"context"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, 0, err
	}
	if response.Context == nil || response.Context.Slot == nil || len(response.Value) == 0 {
		return nil, 0, fmt.Errorf("%s: incomplete account response", method)
	}
	slot := *response.Context.Slot
	if minSlot != nil && slot < *minSlot {
		return nil, 0, fmt.Errorf("account context slot %d precedes %d", slot, *minSlot)
	}
	return response.Value, slot, nil
}

// decodeAccount validates the base64 RPC account shape before defaults can
// turn missing fields into an empty status, invalid recipient, or zero rent.
func decodeAccount(value json.RawMessage) (*rpc.Account, error) {
	var wire *struct {
		Lamports   *uint64           `json:"lamports"`
		Owner      *solana.PublicKey `json:"owner"`
		Data       []*string         `json:"data"`
		Executable *bool             `json:"executable"`
		RentEpoch  *big.Int          `json:"rentEpoch"`
		Space      uint64            `json:"space"`
	}
	if err := json.Unmarshal(value, &wire); err != nil {
		return nil, err
	}
	if wire == nil {
		return nil, nil
	}
	if wire.Lamports == nil || wire.Owner == nil || wire.Executable == nil || wire.RentEpoch == nil {
		return nil, fmt.Errorf("incomplete account fields")
	}
	if len(wire.Data) != 2 || wire.Data[0] == nil || wire.Data[1] == nil || *wire.Data[1] != string(solana.EncodingBase64) {
		return nil, fmt.Errorf("account data must be a base64 tuple")
	}
	data, err := rpc.DataBytesOrJSONFromBase64(*wire.Data[0])
	if err != nil {
		return nil, err
	}
	return &rpc.Account{
		Lamports:   *wire.Lamports,
		Owner:      *wire.Owner,
		Data:       data,
		Executable: *wire.Executable,
		RentEpoch:  wire.RentEpoch,
		Space:      wire.Space,
	}, nil
}

// Account returns nil only for an explicit null value in a valid RPC response.
func (r *Reader) Account(ctx context.Context, address solana.PublicKey, commitment rpc.CommitmentType, minSlot *uint64) (*rpc.Account, uint64, error) {
	value, slot, err := r.accountResponse(ctx, "getAccountInfo", address, commitment, minSlot)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", address, err)
	}
	account, err := decodeAccount(value)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", address, err)
	}
	return account, slot, nil
}

// Accounts requires one explicit result per requested address, including nulls.
func (r *Reader) Accounts(ctx context.Context, addresses []solana.PublicKey, commitment rpc.CommitmentType) ([]*rpc.Account, uint64, error) {
	value, slot, err := r.accountResponse(ctx, "getMultipleAccounts", addresses, commitment, nil)
	if err != nil {
		return nil, 0, err
	}
	var values []json.RawMessage
	if err := json.Unmarshal(value, &values); err != nil {
		return nil, 0, err
	}
	if values == nil || len(values) != len(addresses) {
		return nil, 0, fmt.Errorf("account batch length: got %d, expected %d", len(values), len(addresses))
	}
	accounts := make([]*rpc.Account, len(values))
	for i, value := range values {
		accounts[i], err = decodeAccount(value)
		if err != nil {
			return nil, 0, fmt.Errorf("read %s: %w", addresses[i], err)
		}
	}
	return accounts, slot, nil
}

func owned(info *rpc.Account, program solana.PublicKey) ([]byte, error) {
	if info == nil {
		return nil, fmt.Errorf("required account unavailable")
	}
	if info.Owner != program {
		return nil, fmt.Errorf("account owner mismatch: got %s, expected %s", info.Owner, program)
	}
	if info.Data == nil {
		return nil, fmt.Errorf("account data unavailable")
	}

	return info.Data.GetBinary(), nil
}

func (r *Reader) Follow(ctx context.Context) (Followed, error) {
	deployment, err := Derive(r.program)
	if err != nil {
		return Followed{}, err
	}

	info, slot, err := r.Account(ctx, deployment.Bridge, rpc.CommitmentConfirmed, nil)
	if err != nil {
		return Followed{}, err
	}

	data, err := owned(info, r.program)
	if err != nil {
		return Followed{}, err
	}

	bridge, err := DecodeBridge(data)
	if err != nil {
		return Followed{}, err
	}

	found := Followed{
		Program: bridge.LightClientProgram,
		ChainID: bridge.ChainID,
		Slot:    slot,
	}
	if bridge.ChainID != r.chainID {
		return Followed{}, &FollowMismatch{
			Found:   found,
			Event:   "bridge-chain-mismatch",
			Message: fmt.Sprintf("bridge accepts %s, configured %s", bridge.ChainID, r.chainID),
		}
	}
	if r.pin != nil && bridge.LightClientProgram != *r.pin {
		return Followed{}, &FollowMismatch{
			Found:   found,
			Event:   "light-client-mismatch",
			Message: fmt.Sprintf("bridge light client %s differs from pinned %s", bridge.LightClientProgram, *r.pin),
		}
	}
	appAddress, err := pda(bridge.LightClientProgram, "app_state")
	if err != nil {
		return Followed{}, err
	}

	info, _, err = r.Account(ctx, appAddress, rpc.CommitmentConfirmed, &slot)
	if err != nil {
		return Followed{}, err
	}

	data, err = owned(info, bridge.LightClientProgram)
	if err != nil {
		return Followed{}, err
	}

	app, err := DecodeApp(data)
	if err != nil {
		return Followed{}, err
	}

	f, err := DeriveClient(bridge.LightClientProgram, app.AccessManager)
	if err != nil {
		return Followed{}, err
	}

	f.Slot = slot
	f.ChainID = bridge.ChainID
	return f, nil
}

func (r *Reader) client(ctx context.Context, f Followed, commitment rpc.CommitmentType) (*ClientState, error) {
	info, _, err := r.Account(ctx, f.ClientState, commitment, nil)
	if err != nil || unallocated(info) {
		return nil, err
	}

	data, err := owned(info, f.Program)
	if err != nil {
		return nil, err
	}

	state, err := DecodeClient(data)
	if err != nil {
		return nil, err
	}
	if state.ChainID != r.chainID {
		return nil, fmt.Errorf("client chain ID mismatch")
	}

	return &state, nil
}

func requiredClient(state *ClientState, err error) (ClientState, error) {
	if err != nil {
		return ClientState{}, err
	}
	if state == nil {
		return ClientState{}, fmt.Errorf("client state unavailable")
	}

	return *state, nil
}

func (r *Reader) Client(ctx context.Context, f Followed) (ClientState, error) {
	return requiredClient(r.client(ctx, f, rpc.CommitmentConfirmed))
}

func (r *Reader) ClientFinalized(ctx context.Context, f Followed) (ClientState, error) {
	return requiredClient(r.client(ctx, f, rpc.CommitmentFinalized))
}

func (r *Reader) ClientNullable(ctx context.Context, f Followed) (*ClientState, error) {
	return r.client(ctx, f, rpc.CommitmentConfirmed)
}

type Access string

const (
	AccessMember Access = "member"
	AccessOpen   Access = "open"
	AccessNone   Access = "none"
)

func (r *Reader) AccessState(ctx context.Context, f Followed) (*AccessManager, error) {
	info, _, err := r.Account(ctx, f.AccessState, rpc.CommitmentConfirmed, nil)
	if err != nil || unallocated(info) {
		return nil, err
	}

	data, err := owned(info, f.AccessManager)
	if err != nil {
		return nil, err
	}

	state, err := DecodeAccess(data)
	if err != nil {
		return nil, err
	}

	return &state, nil
}

func (r *Reader) Access(ctx context.Context, f Followed, payer solana.PublicKey) (Access, error) {
	state, err := r.AccessState(ctx, f)
	if err != nil {
		return "", err
	}
	if state == nil {
		return "", fmt.Errorf("access manager state unavailable")
	}

	access := AccessNone
	for _, role := range state.Roles {
		if role.ID != 1 {
			continue
		}
		for _, member := range role.Members {
			if member == payer {
				return AccessMember, nil
			}
			if member == f.Program {
				access = AccessOpen
			}
		}
	}
	return access, nil
}

// Expired compares nanoseconds without narrowing the on-chain u64 period or
// signed Clock to time.Duration. Equality is expired, as in the client.
func Expired(timestamp, trustingSeconds uint64, nowSeconds int64) bool {
	billion := big.NewInt(1_000_000_000)
	expires := new(big.Int).Mul(new(big.Int).SetUint64(trustingSeconds), billion)
	expires.Add(expires, new(big.Int).SetUint64(timestamp))
	now := new(big.Int).Mul(big.NewInt(nowSeconds), billion)
	return now.Cmp(expires) >= 0
}

func (r *Reader) Active(ctx context.Context, f Followed) error {
	state, err := r.Client(ctx, f)
	if err != nil {
		return err
	}

	consensus, err := r.ConsensusConfirmed(ctx, f, state.LatestHeight.RevisionHeight)
	if err != nil {
		return err
	}
	if consensus == nil {
		return fmt.Errorf("latest consensus unavailable")
	}

	now, err := r.Clock(ctx)
	if err != nil {
		return err
	}
	if state.FrozenHeight.RevisionHeight != 0 {
		return fmt.Errorf("light client is frozen")
	}
	if Expired(consensus.Timestamp, state.TrustingPeriod, now) {
		return fmt.Errorf("light client is expired")
	}

	return nil
}

func clockSeconds(info *rpc.Account) (int64, error) {
	if info == nil || info.Data == nil || len(info.Data.GetBinary()) != 40 {
		return 0, fmt.Errorf("Solana clock unavailable")
	}
	d := &decoder{data: info.Data.GetBinary()}
	d.u64() // slot
	d.u64() // epoch start timestamp
	d.u64() // epoch
	d.u64() // leader schedule epoch
	return int64(d.u64()), d.err
}

func (r *Reader) Clock(ctx context.Context) (int64, error) {
	info, _, err := r.Account(ctx, solana.SysVarClockPubkey, rpc.CommitmentConfirmed, nil)
	if err != nil {
		return 0, err
	}

	return clockSeconds(info)
}

func (r *Reader) SlotHashes(ctx context.Context) ([]uint64, error) {
	info, _, err := r.Account(ctx, solana.SysVarSlotHashesPubkey, rpc.CommitmentConfirmed, nil)
	if err != nil {
		return nil, err
	}
	if info == nil || info.Data == nil {
		return nil, fmt.Errorf("slot hashes unavailable")
	}
	d := &decoder{data: info.Data.GetBinary()}
	count := d.u64()
	if d.err != nil {
		return nil, d.err
	}
	if count == 0 || count > 512 || count > uint64(len(d.data)/40) {
		return nil, fmt.Errorf("invalid slot hashes count")
	}

	slots := make([]uint64, int(count))
	for i := range slots {
		slots[i] = d.u64()
		d.hash()
	}
	return slots, d.err
}

func (r *Reader) Status(ctx context.Context, id sink.ID) (*Status, error) {
	deployment, err := Derive(r.program)
	if err != nil {
		return nil, err
	}

	address, err := deployment.Status(id)
	if err != nil {
		return nil, err
	}

	info, _, err := r.Account(ctx, address, rpc.CommitmentConfirmed, nil)
	if err != nil || unallocated(info) {
		return nil, err
	}

	return r.status(info, id)
}

func unallocated(info *rpc.Account) bool {
	return info == nil || (info.Data != nil && len(info.Data.GetBinary()) == 0)
}

func (r *Reader) status(info *rpc.Account, id sink.ID) (*Status, error) {
	data, err := owned(info, r.program)
	if err != nil {
		return nil, err
	}

	state, err := DecodeStatus(data)
	if err != nil {
		return nil, err
	}
	if state.ID != id {
		return nil, fmt.Errorf("status identity mismatch")
	}

	return &state, nil
}

func (r *Reader) Resolved(ctx context.Context, ids []sink.ID, checkpoint func(context.Context) error) ([]bool, error) {
	deployment, err := Derive(r.program)
	if err != nil {
		return nil, err
	}

	resolved := make([]bool, len(ids))
	for start := 0; start < len(ids); start += 100 {
		if checkpoint != nil {
			if err := checkpoint(ctx); err != nil {
				return nil, err
			}
		}
		end := min(start+100, len(ids))
		addresses := make([]solana.PublicKey, end-start)
		for i, id := range ids[start:end] {
			addresses[i], err = deployment.Status(id)
			if err != nil {
				return nil, err
			}
		}
		infos, _, err := r.Accounts(ctx, addresses, rpc.CommitmentConfirmed)
		if err != nil {
			return nil, err
		}
		for i, info := range infos {
			// Discovery treats foreign and empty PDAs as unresolved. Single-status
			// reads reject nonempty foreign accounts before decoding.
			if info != nil && info.Data == nil {
				return nil, fmt.Errorf("status account data unavailable")
			}
			if info == nil || info.Owner != r.program || len(info.Data.GetBinary()) <= 8 {
				continue
			}
			if _, err := r.status(info, ids[start+i]); err != nil {
				return nil, err
			}
			resolved[start+i] = true
		}
		if checkpoint != nil {
			if err := checkpoint(ctx); err != nil {
				return nil, err
			}
		}
	}
	return resolved, nil
}

func (r *Reader) buffer(ctx context.Context, address solana.PublicKey, commitment rpc.CommitmentType, nullable bool) (*ProofBuffer, error) {
	info, _, err := r.Account(ctx, address, commitment, nil)
	if err != nil || info == nil || (nullable && unallocated(info)) {
		return nil, err
	}

	data, err := owned(info, r.program)
	if err != nil {
		return nil, err
	}

	value, err := DecodeProofBuffer(data)
	if err != nil {
		return nil, err
	}

	expected, err := pda(r.program, "proof", value.Submitter[:], value.Nonce[:])
	if err != nil {
		return nil, err
	}
	if expected != address {
		return nil, fmt.Errorf("proof buffer identity mismatch")
	}

	value.Lamports = info.Lamports
	return &value, nil
}

func (r *Reader) Buffer(ctx context.Context, address solana.PublicKey) (*ProofBuffer, error) {
	return r.buffer(ctx, address, rpc.CommitmentFinalized, false)
}

func (r *Reader) BufferConfirmed(ctx context.Context, address solana.PublicKey) (*ProofBuffer, error) {
	return r.buffer(ctx, address, rpc.CommitmentConfirmed, true)
}

func (r *Reader) consensus(ctx context.Context, f Followed, height uint64, commitment rpc.CommitmentType) (*Consensus, error) {
	address, err := f.ConsensusAt(height)
	if err != nil {
		return nil, err
	}

	info, _, err := r.Account(ctx, address, commitment, nil)
	if err != nil || unallocated(info) {
		return nil, err
	}

	data, err := owned(info, f.Program)
	if err != nil {
		return nil, err
	}

	value, err := DecodeConsensus(data)
	if err != nil {
		return nil, err
	}
	if value.Height != height {
		return nil, fmt.Errorf("consensus height mismatch")
	}

	value.Address = address
	value.Lamports = info.Lamports
	return &value, nil
}

func (r *Reader) Consensus(ctx context.Context, f Followed, height uint64) (*Consensus, error) {
	return r.consensus(ctx, f, height, rpc.CommitmentFinalized)
}

func (r *Reader) ConsensusConfirmed(ctx context.Context, f Followed, height uint64) (*Consensus, error) {
	return r.consensus(ctx, f, height, rpc.CommitmentConfirmed)
}

func (r *Reader) lookup(ctx context.Context, address solana.PublicKey, commitment rpc.CommitmentType) (*Lookup, error) {
	info, _, err := r.Account(ctx, address, commitment, nil)
	if err != nil || info == nil {
		return nil, err
	}

	data, err := owned(info, solana.AddressLookupTableProgramID)
	if err != nil {
		return nil, err
	}
	// The pinned decoder accepts partial addresses; web3.js rejects them.
	if len(data) < 56 || (len(data)-56)%32 != 0 {
		return nil, fmt.Errorf("invalid lookup table size")
	}

	state, err := addresslookuptable.DecodeAddressLookupTableState(data)
	if err != nil {
		return nil, err
	}
	if state.TypeIndex != 1 {
		return nil, fmt.Errorf("invalid lookup table type")
	}

	return &Lookup{
		Address:  address,
		State:    *state,
		Lamports: info.Lamports,
	}, nil
}

func (r *Reader) Lookup(ctx context.Context, address solana.PublicKey) (*Lookup, error) {
	return r.lookup(ctx, address, rpc.CommitmentFinalized)
}

func (r *Reader) LookupConfirmed(ctx context.Context, address solana.PublicKey) (*Lookup, error) {
	return r.lookup(ctx, address, rpc.CommitmentConfirmed)
}

// HeaderTemporaryInitializable reports whether a present account can be allocated
// for a header upload. Nil remains explicit absence; undecoded data is not empty.
func HeaderTemporaryInitializable(info *rpc.Account) bool {
	return info != nil && !info.Executable && info.Owner == solana.SystemProgramID &&
		info.Data != nil && info.Data.GetBinary() != nil && len(info.Data.GetBinary()) == 0
}

// Chunk returns nil when there is no allocated header chunk.
func (r *Reader) Chunk(ctx context.Context, f Followed, payer solana.PublicKey, target uint64, index uint8) (*HeaderChunk, error) {
	address, err := f.ChunkAt(payer, target, index)
	if err != nil {
		return nil, err
	}

	info, _, err := r.Account(ctx, address, rpc.CommitmentConfirmed, nil)
	if err != nil || info == nil {
		return nil, err
	}
	if HeaderTemporaryInitializable(info) {
		return nil, nil
	}
	if info.Executable {
		return nil, fmt.Errorf("executable header upload account")
	}

	data, err := owned(info, f.Program)
	if err != nil {
		return nil, err
	}

	value, err := DecodeChunk(data)
	if err != nil {
		return nil, err
	}
	if value.Submitter != payer {
		return nil, fmt.Errorf("header chunk submitter mismatch")
	}

	value.Lamports = info.Lamports
	return &value, nil
}

// Batch returns nil when there is no allocated signature batch.
func (r *Reader) Batch(ctx context.Context, f Followed, payer solana.PublicKey, session [32]byte) (*SignatureBatch, error) {
	address, err := f.BatchAt(payer, session)
	if err != nil {
		return nil, err
	}

	info, _, err := r.Account(ctx, address, rpc.CommitmentConfirmed, nil)
	if err != nil || info == nil {
		return nil, err
	}
	if HeaderTemporaryInitializable(info) {
		return nil, nil
	}
	if info.Executable {
		return nil, fmt.Errorf("executable header upload account")
	}

	data, err := owned(info, f.Program)
	if err != nil {
		return nil, err
	}

	value, err := DecodeBatch(data)
	if err != nil {
		return nil, err
	}
	if value.Version != 1 || value.Submitter != payer || value.Session != session {
		return nil, fmt.Errorf("signature batch identity mismatch")
	}

	value.Lamports = info.Lamports
	return &value, nil
}

func (r *Reader) Launch(ctx context.Context) (*LaunchConfig, error) {
	d, err := Derive(r.program)
	if err != nil {
		return nil, err
	}

	info, _, err := r.Account(ctx, d.Config, rpc.CommitmentConfirmed, nil)
	if err != nil || unallocated(info) {
		return nil, err
	}

	data, err := owned(info, r.program)
	if err != nil {
		return nil, err
	}

	value, err := DecodeLaunch(data)
	if err != nil {
		return nil, err
	}

	return &value, nil
}

func (r *Reader) Bridge(ctx context.Context) (*BridgeConfig, error) {
	d, err := Derive(r.program)
	if err != nil {
		return nil, err
	}

	info, _, err := r.Account(ctx, d.Bridge, rpc.CommitmentConfirmed, nil)
	if err != nil || unallocated(info) {
		return nil, err
	}

	data, err := owned(info, r.program)
	if err != nil {
		return nil, err
	}

	value, err := DecodeBridge(data)
	if err != nil {
		return nil, err
	}

	return &value, nil
}

func (r *Reader) ConsensusStates(ctx context.Context, f Followed) ([]Consensus, error) {
	options := rpc.M{
		"commitment": rpc.CommitmentConfirmed,
		"encoding":   solana.EncodingBase64,
		"filters": []rpc.RPCFilter{
			{DataSize: 88},
			{
				Memcmp: &rpc.RPCFilterMemcmp{
					Offset: 0,
					Bytes:  solana.Base58(discriminator("account", "ConsensusStateStore")),
				},
			},
		},
	}
	var raw json.RawMessage
	if err := r.rpc.RPCCallForInto(ctx, &raw, "getProgramAccounts", []interface{}{f.Program, options}); err != nil {
		return nil, err
	}
	var accounts []*struct {
		Pubkey  *solana.PublicKey `json:"pubkey"`
		Account json.RawMessage   `json:"account"`
	}
	if err := json.Unmarshal(raw, &accounts); err != nil {
		return nil, err
	}
	if accounts == nil {
		return nil, fmt.Errorf("program accounts result unavailable")
	}

	states := make([]Consensus, 0, len(accounts))
	for _, account := range accounts {
		if account == nil || account.Pubkey == nil {
			return nil, fmt.Errorf("missing program account")
		}
		info, err := decodeAccount(account.Account)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", account.Pubkey, err)
		}
		data, err := owned(info, f.Program)
		if err != nil {
			return nil, err
		}
		if len(data) != 88 {
			return nil, fmt.Errorf("invalid consensus store size")
		}
		state, err := DecodeConsensus(data)
		if err != nil {
			return nil, err
		}
		// Keep the scan count intact; the pruning caller excludes foreign PDAs.
		state.Address = *account.Pubkey
		state.Lamports = info.Lamports
		states = append(states, state)
	}
	return states, nil
}
