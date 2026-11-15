package bridge

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"filippo.io/edwards25519"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	sink "pkg.akt.dev/go/node/sink/v1"
)

type RecipientView struct {
	Recipient solana.PublicKey
	ATA       solana.PublicKey
	Mints     bool
}

type MintView struct {
	Slot           uint64
	Enabled        bool
	PerTransferMax uint64
	DailyMintCap   uint64
	MintDay        uint64
	MintedToday    uint64
	NowSeconds     int64
	Recipients     []RecipientView
}

func (r *Reader) MintView(ctx context.Context, records []sink.RecordData, checkpoint func(context.Context) error) (MintView, error) {
	d, err := Derive(r.program)
	if err != nil {
		return MintView{}, err
	}
	infos, slot, err := r.Accounts(ctx, []solana.PublicKey{d.Config, d.Bridge, solana.SysVarClockPubkey}, rpc.CommitmentConfirmed)
	if err != nil {
		return MintView{}, err
	}

	data, err := owned(infos[0], r.program)
	if err != nil {
		return MintView{}, err
	}

	launch, err := DecodeLaunch(data)
	if err != nil {
		return MintView{}, err
	}

	data, err = owned(infos[1], r.program)
	if err != nil {
		return MintView{}, err
	}

	bridge, err := DecodeBridge(data)
	if err != nil {
		return MintView{}, err
	}

	now, err := clockSeconds(infos[2])
	if err != nil {
		return MintView{}, err
	}

	view := MintView{
		Slot:           slot,
		Enabled:        launch.BridgeEnabled,
		PerTransferMax: bridge.Policy.PerTransferMax,
		DailyMintCap:   bridge.Policy.DailyMintCap,
		MintDay:        bridge.MintDay,
		MintedToday:    bridge.MintedToday,
		NowSeconds:     now,
		Recipients:     make([]RecipientView, len(records)),
	}
	for start := 0; start < len(records); start += 50 {
		if checkpoint != nil {
			if err := checkpoint(ctx); err != nil {
				return MintView{}, err
			}
		}
		end := min(start+50, len(records))
		addresses := make([]solana.PublicKey, 0, 2*(end-start))
		for i := start; i < end; i++ {
			recipient, err := solana.PublicKeyFromBase58(records[i].Dest)
			if err != nil {
				return MintView{}, fmt.Errorf("recipient: %w", err)
			}
			ata, err := AssociatedToken(recipient, d.Mint)
			if err != nil {
				return MintView{}, err
			}
			view.Recipients[i] = RecipientView{
				Recipient: recipient,
				ATA:       ata,
			}
			addresses = append(addresses, recipient, ata)
		}
		infos, _, err := r.Accounts(ctx, addresses, rpc.CommitmentConfirmed)
		if err != nil {
			return MintView{}, err
		}
		for i := start; i < end; i++ {
			j := 2 * (i - start)
			view.Recipients[i].Mints = recipientMints(view.Recipients[i].Recipient, infos[j], infos[j+1], d, bridge.Policy.Vaults)
		}
		if checkpoint != nil {
			if err := checkpoint(ctx); err != nil {
				return MintView{}, err
			}
		}
	}
	return view, nil
}

// noble's strict point decoding (used by the baseline predictor) rejects
// noncanonical encodings accepted by solana-go's IsOnCurve.
func canonicalOnCurve(key solana.PublicKey) bool {
	point, err := new(edwards25519.Point).SetBytes(key[:])
	return err == nil && bytes.Equal(point.Bytes(), key[:])
}

func recipientMints(recipient solana.PublicKey, account, ata *rpc.Account, d Deployment, vaults []solana.PublicKey) bool {
	owner := solana.SystemProgramID
	if account != nil {
		owner = account.Owner
	}
	if (account != nil && account.Executable) || recipient.IsZero() || recipient == d.MintAuthority || owner == solana.TokenProgramID || owner == solana.Token2022ProgramID {
		return false
	}

	wallet := owner == solana.SystemProgramID && canonicalOnCurve(recipient)
	return (wallet || slices.Contains(vaults, recipient)) && ataUsable(ata, d.Mint, recipient)
}

func ataUsable(info *rpc.Account, mint, recipient solana.PublicKey) bool {
	if info == nil {
		return true
	}
	if info.Data == nil {
		return false
	}

	data := info.Data.GetBinary()
	if info.Owner == solana.SystemProgramID {
		return len(data) == 0
	}
	if info.Owner != solana.TokenProgramID || len(data) != 165 {
		return false
	}
	d := &decoder{data: data}
	storedMint, owner := d.key(), d.key()
	d.u64()    // balance
	d.take(36) // optional delegate
	state := d.u8()
	return storedMint == mint && owner == recipient && state == 1
}
