package bridge

import (
	"github.com/gagliardetto/solana-go"

	"pkg.akt.dev/node/v2/internal/relayer/header"
)

func InitializeAccess(program, payer, authority, admin solana.PublicKey) (solana.Instruction, error) {
	state, err := pda(program, "access_manager")
	if err != nil {
		return nil, err
	}

	programData, err := ProgramData(program)
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("initialize")
	e.key(admin)
	return ix(program, e,
		write(state),
		sign(payer, true),
		read(solana.SystemProgramID),
		read(solana.SysVarInstructionsPubkey),
		read(programData),
		sign(authority, false),
	), nil
}

func GrantRole(program, admin, member solana.PublicKey, role uint64) (solana.Instruction, error) {
	state, err := pda(program, "access_manager")
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("grant_role")
	e.u64(role)
	e.key(member)
	return ix(program, e, write(state), sign(admin, false), read(solana.SysVarInstructionsPubkey)), nil
}

func (d Deployment) Initialize(authority solana.PublicKey) (solana.Instruction, error) {
	programData, err := ProgramData(d.Program)
	if err != nil {
		return nil, err
	}

	return ix(d.Program, instructionEncoder("initialize"),
		read(d.Program),
		read(programData),
		sign(authority, true),
		write(d.Config),
		write(d.Mint),
		read(d.MintAuthority),
		read(solana.TokenProgramID),
		read(solana.SystemProgramID),
	), nil
}

func (f Followed) Initialize(payer, authority solana.PublicKey, state ClientState, packet header.Packet) (solana.Instruction, error) {
	programData, err := ProgramData(f.Program)
	if err != nil {
		return nil, err
	}

	consensus, err := f.ConsensusAt(state.LatestHeight.RevisionHeight)
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("initialize")
	e.client(state)
	e.u64(packet.Timestamp)
	e.hash(packet.Root)
	e.hash(packet.NextValidatorsHash)
	e.key(f.AccessManager)
	return ix(f.Program, e,
		write(f.ClientState),
		write(consensus),
		write(f.AppState),
		sign(payer, true),
		read(solana.SystemProgramID),
		read(programData),
		sign(authority, false),
	), nil
}

func (d Deployment) ConfigureBridge(authority solana.PublicKey, f Followed, chainID string, policy MintPolicy) (solana.Instruction, error) {
	programData, err := ProgramData(d.Program)
	if err != nil {
		return nil, err
	}

	consensus, err := f.ConsensusAt(f.Latest)
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("configure_bridge")
	e.str(chainID)
	e.policy(policy)
	return ix(d.Program, e,
		read(d.Program),
		read(programData),
		sign(authority, true),
		read(d.Config),
		write(d.Bridge),
		read(f.Program),
		read(f.ClientState),
		read(consensus),
		read(solana.SystemProgramID),
	), nil
}

func (d Deployment) SetBridgeEnabled(authority solana.PublicKey, enabled bool) (solana.Instruction, error) {
	programData, err := ProgramData(d.Program)
	if err != nil {
		return nil, err
	}

	e := instructionEncoder("set_bridge_enabled")
	e.boolean(enabled)
	return ix(d.Program, e, read(d.Program), read(programData), sign(authority, true), write(d.Config), write(d.Bridge)), nil
}
