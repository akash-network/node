package bridge

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"

	"pkg.akt.dev/node/v2/internal/relayer/header"
)

var ed25519ProgramID = solana.MustPublicKeyFromBase58("Ed25519SigVerify111111111111111111111111111")

func Compute() []solana.Instruction {
	return []solana.Instruction{
		computebudget.NewSetComputeUnitLimitInstruction(1_400_000).Build(),
		computebudget.NewRequestHeapFrameInstruction(262_144).Build(),
	}
}

// Ed25519 must be instruction zero in the receipt transaction. All offsets
// reference this instruction's data, as required by the ICS07 verifier.
func Ed25519(receipt header.Receipt) (solana.Instruction, error) {
	if len(receipt.Message) > math.MaxUint16 {
		return nil, fmt.Errorf("Ed25519 message exceeds u16 length")
	}

	data := []byte{1, 0}
	for _, value := range []uint16{48, math.MaxUint16, 16, math.MaxUint16, 112, uint16(len(receipt.Message)), math.MaxUint16} {
		data = binary.LittleEndian.AppendUint16(data, value)
	}
	data = append(data, receipt.PubKey[:]...)
	data = append(data, receipt.Signature[:]...)
	data = append(data, receipt.Message...)
	return solana.NewInstruction(ed25519ProgramID, nil, data), nil
}

func CreateLookup(authority, payer solana.PublicKey, recentSlot uint64) (solana.Instruction, solana.PublicKey, error) {
	address, bump, err := solana.FindProgramAddress([][]byte{authority[:], le64(recentSlot)}, solana.AddressLookupTableProgramID)
	if err != nil {
		return nil, solana.PublicKey{}, err
	}

	e := encoder{}
	e.u32(0)
	e.u64(recentSlot)
	e.u8(bump)
	return ix(solana.AddressLookupTableProgramID, e, write(address), sign(authority, false), sign(payer, true), read(solana.SystemProgramID)), address, nil
}

func ExtendLookup(table, authority, payer solana.PublicKey, addresses []solana.PublicKey) (solana.Instruction, error) {
	e := encoder{}
	e.u32(2)
	e.u64(uint64(len(addresses)))
	for _, address := range addresses {
		e.key(address)
	}
	return ix(solana.AddressLookupTableProgramID, e,
		write(table),
		sign(authority, false),
		sign(payer, true),
		read(solana.SystemProgramID),
	), nil
}

func DeactivateLookup(table, authority solana.PublicKey) (solana.Instruction, error) {
	e := encoder{}
	e.u32(3)
	return ix(solana.AddressLookupTableProgramID, e, write(table), sign(authority, false)), nil
}

func CloseLookup(table, authority, recipient solana.PublicKey) (solana.Instruction, error) {
	e := encoder{}
	e.u32(4)
	return ix(solana.AddressLookupTableProgramID, e, write(table), sign(authority, false), write(recipient)), nil
}

func Compile(payer solana.PublicKey, blockhash solana.Hash, instructions []solana.Instruction, tables map[solana.PublicKey]solana.PublicKeySlice) (*solana.Transaction, int, error) {
	transaction, err := solana.NewTransaction(instructions, blockhash, solana.TransactionPayer(payer), solana.TransactionAddressTables(tables))
	if err != nil {
		return nil, 0, err
	}

	transaction.Message.SetVersion(solana.MessageVersionV0)
	accounts := len(transaction.Message.AccountKeys) + transaction.Message.AddressTableLookups.NumLookups()
	if accounts > 64 {
		return nil, accounts, fmt.Errorf("transaction uses %d accounts, maximum is 64", accounts)
	}

	return transaction, accounts, nil
}
