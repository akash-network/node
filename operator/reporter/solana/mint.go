package solana

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// mintSize is the length of an SPL Token Mint account.
const mintSize = 82

// Mint is an SPL Token mint account.
type Mint struct {
	MintAuthority   *solana.PublicKey
	Supply          uint64
	Decimals        uint8
	Initialized     bool
	FreezeAuthority *solana.PublicKey
}

// coption reads an SPL COption<Pubkey>: a u32 tag, then the key, which is present in both cases.
func (r *reader) coption() *solana.PublicKey {
	tag := r.u32()
	k := r.key()

	switch tag {
	case 0:
		return nil
	case 1:
		if r.err != nil {
			return nil
		}

		return &k
	default:
		r.fail(fmt.Errorf("invalid COption tag %d", tag))

		return nil
	}
}

// DecodeMint decodes an SPL Token mint account, which is exactly 82 bytes.
func DecodeMint(data []byte) (Mint, error) {
	if len(data) != mintSize {
		return Mint{}, fmt.Errorf("decode Mint: %d bytes, want %d", len(data), mintSize)
	}

	r := reader{data: data}
	mint := Mint{
		MintAuthority:   r.coption(),
		Supply:          r.u64(),
		Decimals:        r.u8(),
		Initialized:     r.bool(),
		FreezeAuthority: r.coption(),
	}
	if r.err != nil {
		return Mint{}, fmt.Errorf("decode Mint: %w", r.err)
	}

	return mint, nil
}
