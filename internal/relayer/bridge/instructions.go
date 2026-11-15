package bridge

import (
	"crypto/sha256"
	"fmt"

	"github.com/gagliardetto/solana-go"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/header"
)

func read(key solana.PublicKey) *solana.AccountMeta  { return solana.Meta(key) }
func write(key solana.PublicKey) *solana.AccountMeta { return solana.Meta(key).WRITE() }
func sign(key solana.PublicKey, writable bool) *solana.AccountMeta {
	meta := solana.Meta(key).SIGNER()
	if writable {
		meta.WRITE()
	}
	return meta
}

func ix(program solana.PublicKey, data encoder, accounts ...*solana.AccountMeta) solana.Instruction {
	return solana.NewInstruction(program, solana.AccountMetaSlice(accounts), []byte(data))
}

func (d Deployment) CreateProofBuffer(payer solana.PublicKey, key []byte, expectedLen uint32) (solana.Instruction, error) {
	address, err := d.ProofBuffer(payer, key)
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("create_proof_buffer")
	e.hash(sha256.Sum256(key))
	e.u32(expectedLen)
	return ix(d.Program, e, sign(payer, true), write(address), read(solana.SystemProgramID)), nil
}

func (d Deployment) GrowProofBuffer(payer, address solana.PublicKey) (solana.Instruction, error) {
	return ix(d.Program, instructionEncoder("grow_proof_buffer"),
		sign(payer, true),
		write(address),
		read(solana.SystemProgramID),
	), nil
}

func (d Deployment) AppendProof(payer, address solana.PublicKey, offset uint32, chunk []byte) (solana.Instruction, error) {
	e := instructionEncoder("append_proof")
	e.u32(offset)
	e.vector(chunk)
	return ix(d.Program, e, sign(payer, false), write(address)), nil
}

func (d Deployment) CloseProofBuffer(payer, address solana.PublicKey) (solana.Instruction, error) {
	return ix(d.Program, instructionEncoder("close_proof_buffer"), sign(payer, true), write(address)), nil
}

type MintArgs struct {
	Payer       solana.PublicKey
	Client      Followed
	ID          sink.ID
	ProofBuffer solana.PublicKey
	Recipient   solana.PublicKey
	Amount      uint64
	ProofHeight uint64
}

type VoidArgs struct {
	Payer            solana.PublicKey
	Client           Followed
	ID               sink.ID
	ProofBuffer      solana.PublicKey
	SettlementStatus uint8
	ProofHeight      uint64
}

func (d Deployment) MintTransfer(args MintArgs) (solana.Instruction, error) {
	status, err := d.Status(args.ID)
	if err != nil {
		return nil, err
	}

	latest, err := args.Client.ConsensusAt(args.Client.Latest)
	if err != nil {
		return nil, err
	}

	proof, err := args.Client.ConsensusAt(args.ProofHeight)
	if err != nil {
		return nil, err
	}

	ata, err := AssociatedToken(args.Recipient, d.Mint)
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("mint_transfer")
	if err := e.id(args.ID); err != nil {
		return nil, err
	}

	e.key(args.Recipient)
	e.u64(args.Amount)
	e.u64(args.ProofHeight)
	return ix(d.Program, e,
		sign(args.Payer, true),
		read(d.Config),
		write(d.Bridge),
		read(args.ProofBuffer),
		write(status),
		read(args.Client.Program),
		read(args.Client.ClientState),
		read(latest),
		read(proof),
		write(d.Mint),
		read(d.MintAuthority),
		read(args.Recipient),
		write(ata),
		read(solana.TokenProgramID),
		read(solana.SPLAssociatedTokenAccountProgramID),
		read(solana.SystemProgramID),
	), nil
}

func (d Deployment) VoidTransfer(args VoidArgs) (solana.Instruction, error) {
	status, err := d.Status(args.ID)
	if err != nil {
		return nil, err
	}

	latest, err := args.Client.ConsensusAt(args.Client.Latest)
	if err != nil {
		return nil, err
	}

	proof, err := args.Client.ConsensusAt(args.ProofHeight)
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("void_transfer")
	if err := e.id(args.ID); err != nil {
		return nil, err
	}

	e.u8(args.SettlementStatus)
	e.u64(args.ProofHeight)
	return ix(d.Program, e,
		sign(args.Payer, true),
		read(d.Bridge),
		read(args.ProofBuffer),
		write(status),
		read(args.Client.Program),
		read(args.Client.ClientState),
		read(latest),
		read(proof),
		read(solana.SystemProgramID),
	), nil
}

func (f Followed) InitializeSignatureBatch(payer solana.PublicKey, session [32]byte) (solana.Instruction, error) {
	batch, err := f.BatchAt(payer, session)
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("initialize_signature_batch")
	e.hash(session)
	return ix(f.Program, e,
		write(batch),
		read(f.AppState),
		read(f.AccessState),
		sign(payer, true),
		read(solana.SysVarInstructionsPubkey),
		read(solana.SystemProgramID),
	), nil
}

func (f Followed) PreVerifySignatureBatch(payer, batch solana.PublicKey, receipt header.Receipt) (solana.Instruction, error) {
	e := instructionEncoder("pre_verify_signature_batch")
	e.hash(receipt.Hash)
	e.hash(receipt.PubKey)
	e.vector(receipt.Message)
	e.raw(receipt.Signature[:])
	return ix(f.Program, e,
		write(batch),
		read(f.AppState),
		read(f.AccessState),
		sign(payer, false),
		read(solana.SysVarInstructionsPubkey),
	), nil
}

func (f Followed) UploadHeaderChunk(payer solana.PublicKey, target uint64, index uint8, data []byte) (solana.Instruction, error) {
	chunk, err := f.ChunkAt(payer, target, index)
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("upload_header_chunk")
	e.u64(target)
	e.u8(index)
	e.vector(data)
	return ix(f.Program, e,
		write(chunk),
		read(f.ClientState),
		read(f.AppState),
		read(f.AccessState),
		sign(payer, true),
		read(solana.SysVarInstructionsPubkey),
		read(solana.SystemProgramID),
	), nil
}

func (f Followed) Assemble(payer solana.PublicKey, target, trusted uint64, chunks, batches []solana.PublicKey) (solana.Instruction, error) {
	if len(chunks) == 0 || len(chunks) > 255 {
		return nil, fmt.Errorf("assembly requires 1 to 255 chunks")
	}

	trust, err := f.ConsensusAt(trusted)
	if err != nil {
		return nil, err
	}

	newState, err := f.ConsensusAt(target)
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("assemble_and_update_client")
	e.u64(target)
	e.u8(uint8(len(chunks)))
	e.u64(trusted)
	metas := solana.AccountMetaSlice{
		write(f.ClientState),
		read(f.AppState),
		read(f.AccessState),
		read(trust),
		write(newState),
		sign(payer, true),
		read(solana.SystemProgramID),
		read(solana.SysVarInstructionsPubkey),
	}
	for _, key := range chunks {
		metas = append(metas, write(key))
	}
	for _, key := range batches {
		metas = append(metas, write(key))
	}
	return ix(f.Program, e, metas...), nil
}

func (f Followed) Cleanup(payer solana.PublicKey, accounts []solana.PublicKey) (solana.Instruction, error) {
	metas := solana.AccountMetaSlice{sign(payer, true)}
	for _, key := range accounts {
		metas = append(metas, write(key))
	}
	return ix(f.Program, instructionEncoder("cleanup_incomplete_upload"), metas...), nil
}

func (f Followed) Prune(payer solana.PublicKey, height uint64) (solana.Instruction, error) {
	consensus, err := f.ConsensusAt(height)
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("prune_consensus_state")
	e.u64(height)
	return ix(f.Program, e,
		read(f.ClientState),
		read(f.AppState),
		read(f.AccessState),
		write(consensus),
		sign(payer, true),
		read(solana.SysVarInstructionsPubkey),
	), nil
}
