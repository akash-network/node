// Package simnode simulates the Akash and Solana nodes the reporter reads, serving a Scenario over the nodes' own
// protocols for integration tests, goldens and benchmarks.
package simnode

import (
	"encoding/json"
	"time"

	"github.com/gagliardetto/solana-go"
)

// Scenario is the chain state a Node serves. It round-trips through encoding/json. Every time in it is an offset
// from the moment Start runs, so the state is equally fresh whenever and wherever it is served.
type Scenario struct {
	// Passes is how many passes the goldens run; zero means one.
	Passes int    `json:"passes,omitempty"`
	Solana Solana `json:"solana"`
}

// Solana is the finalized state of the Solana cluster.
type Solana struct {
	GenesisHash string `json:"genesisHash"`
	// Slot is the finalized slot: the context slot of account reads and the newest block served.
	Slot uint64 `json:"slot"`
	// BlockTimes holds each slot's block time as an offset from start. getBlockTime answers null for other slots.
	BlockTimes map[uint64]time.Duration     `json:"blockTimes,omitempty"`
	Accounts   map[solana.PublicKey]Account `json:"accounts,omitempty"`
	Blocks     map[uint64]Block             `json:"blocks,omitempty"`
	Failures   map[string]Failure           `json:"failures,omitempty"`
}

// Account is an account as the finalized bank holds it.
type Account struct {
	Owner      solana.PublicKey `json:"owner"`
	Executable bool             `json:"executable,omitempty"`
	Lamports   uint64           `json:"lamports,omitempty"`
	Data       []byte           `json:"data,omitempty"`
}

// Block is a confirmed block. Its hashes and parent are derived from its slot; its time comes from BlockTimes.
type Block struct {
	Transactions []Transaction `json:"transactions"`
}

// Transaction is a transaction as Agave's JSON encoding describes it.
type Transaction struct {
	// Signatures are base58 text; they are not checked, so a scenario can serve malformed ones.
	Signatures []string `json:"signatures"`
	// Version is "legacy" (also when empty) or a decimal version such as "0" or "1".
	Version     string             `json:"version,omitempty"`
	Header      Header             `json:"header"`
	AccountKeys []solana.PublicKey `json:"accountKeys"`
	// AddressTableLookups, LoadedWritable and LoadedReadonly describe a v0 message's lookup tables.
	AddressTableLookups []AddressTableLookup `json:"addressTableLookups,omitempty"`
	LoadedWritable      []solana.PublicKey   `json:"loadedWritable,omitempty"`
	LoadedReadonly      []solana.PublicKey   `json:"loadedReadonly,omitempty"`
	// Err is meta.err, a TransactionError in JSON; empty means success.
	Err json.RawMessage `json:"err,omitempty"`
	// Logs is meta.logMessages; nil serves null, as a node that recorded no logs does.
	Logs []string `json:"logs"`
}

// Header is a message header.
type Header struct {
	NumRequiredSignatures       int `json:"numRequiredSignatures"`
	NumReadonlySignedAccounts   int `json:"numReadonlySignedAccounts"`
	NumReadonlyUnsignedAccounts int `json:"numReadonlyUnsignedAccounts"`
}

// AddressTableLookup is a v0 message's reference to an address lookup table.
type AddressTableLookup struct {
	AccountKey      solana.PublicKey `json:"accountKey"`
	WritableIndexes []int            `json:"writableIndexes"`
	ReadonlyIndexes []int            `json:"readonlyIndexes"`
}

// Failure makes a JSON-RPC method fail. A non-zero HTTPStatus answers with that status and no body; otherwise the
// method returns the JSON-RPC error Code and Message.
type Failure struct {
	Code       int    `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	// Times is how many calls fail before the failure is removed from the scenario; zero fails every call.
	Times int `json:"times,omitempty"`
}
