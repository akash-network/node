package solana

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// PDA returns the program address for seeds. It panics when a seed is longer than 32 bytes or there are more than 16 seeds,
// which are programming errors.
func PDA(program solana.PublicKey, seeds ...[]byte) solana.PublicKey {
	addr, _, err := solana.FindProgramAddress(seeds, program)
	if err != nil {
		panic(fmt.Sprintf("solana: derive program address: %v", err))
	}

	return addr
}

// StatusAddress returns the TransferStatus address ["transfer", SHA256(owner), LE64(height), LE64(sequence)].
func StatusAddress(program solana.PublicKey, id sinkid.ID) solana.PublicKey {
	owner := sha256.Sum256(id.OwnerBytes)

	return PDA(program,
		[]byte("transfer"),
		owner[:],
		binary.LittleEndian.AppendUint64(nil, uint64(id.Height)), //nolint:gosec // sink heights are positive int64
		binary.LittleEndian.AppendUint64(nil, id.Sequence),
	)
}
