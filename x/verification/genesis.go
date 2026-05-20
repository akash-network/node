package verification

import (
	"encoding/json"

	errorsmod "cosmossdk.io/errors"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	"pkg.akt.dev/node/v2/x/verification/keeper"
	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
)

func ValidateGenesis(data *vtypes.GenesisState) error {
	seenAuditors := make(map[string]struct{}, len(data.Auditors))
	for _, record := range data.Auditors {
		if _, err := sdk.AccAddressFromBech32(record.Address); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrap("verification: invalid auditor address")
		}
		if _, exists := seenAuditors[record.Address]; exists {
			return errorsmod.Wrap(moduletypes.ErrInvalidReason, "verification: duplicate auditor")
		}
		seenAuditors[record.Address] = struct{}{}
		if err := validateTier(record.MaxAttestationTier); err != nil {
			return err
		}
	}

	seenAttestations := make(map[string]struct{}, len(data.Attestations))
	for _, record := range data.Attestations {
		if err := validateProviderAuditor(record.Provider, record.Auditor); err != nil {
			return err
		}
		key := record.Provider + "/" + record.Auditor
		if _, exists := seenAttestations[key]; exists {
			return errorsmod.Wrap(moduletypes.ErrInvalidReason, "verification: duplicate attestation")
		}
		seenAttestations[key] = struct{}{}
		if err := validateTier(record.Tier); err != nil {
			return err
		}
	}

	for _, record := range data.AuditEscrows {
		if _, err := sdk.AccAddressFromBech32(record.Provider); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrap("verification: invalid audit escrow provider address")
		}
		if err := validateTier(record.RequestedTier); err != nil {
			return err
		}
	}

	for _, record := range data.Discrepancies {
		if err := validateProviderAuditor(record.Provider, record.AuditorA); err != nil {
			return err
		}
		if err := validateProviderAuditor(record.Provider, record.AuditorB); err != nil {
			return err
		}
		if record.AuditorA == record.AuditorB {
			return errorsmod.Wrap(moduletypes.ErrSelfAttestation, "verification: discrepancy auditors match")
		}
		if err := validateTier(record.AuditorATier); err != nil {
			return err
		}
		if err := validateTier(record.AuditorBTier); err != nil {
			return err
		}
	}

	for _, record := range data.ProviderBonds {
		if _, err := sdk.AccAddressFromBech32(record.Provider); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrap("verification: invalid provider bond address")
		}
	}

	for _, record := range data.ProviderSnapshots {
		if _, err := sdk.AccAddressFromBech32(record.Provider); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrap("verification: invalid provider snapshot address")
		}
	}

	for _, record := range data.VerificationGraces {
		if _, err := sdk.AccAddressFromBech32(record.Provider); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrap("verification: invalid grace provider address")
		}
		if err := validateTier(record.PreservedTier); err != nil {
			return err
		}
	}

	return nil
}

func InitGenesis(ctx sdk.Context, k keeper.Keeper, data *vtypes.GenesisState) {
	if err := ValidateGenesis(data); err != nil {
		panic(err)
	}

	k.SetParams(ctx, data.Params)

	for _, record := range data.Auditors {
		if err := k.SetAuditor(ctx, record); err != nil {
			panic(err)
		}
	}
	for _, record := range data.Attestations {
		if err := k.SetAttestation(ctx, record); err != nil {
			panic(err)
		}
	}
	for _, record := range data.Discrepancies {
		k.SetDiscrepancy(ctx, record)
	}
	for _, record := range data.ProviderBonds {
		if err := k.SetProviderBond(ctx, record); err != nil {
			panic(err)
		}
	}
	for _, record := range data.ProviderSnapshots {
		if err := k.SetProviderSnapshot(ctx, record); err != nil {
			panic(err)
		}
	}
	for _, record := range data.AuditEscrows {
		if err := k.SetAuditEscrow(ctx, record); err != nil {
			panic(err)
		}
	}
	for _, record := range data.VerificationGraces {
		if err := k.SetProviderVerificationGrace(ctx, record); err != nil {
			panic(err)
		}
	}

	k.SetNextDiscrepancyID(ctx, data.NextDiscrepancyID)
	k.SetNextAuditEscrowID(ctx, data.NextAuditEscrowID)
	k.SetNextGraceRecordID(ctx, data.NextGraceRecordID)
}

func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *vtypes.GenesisState {
	state := &vtypes.GenesisState{
		Params:            k.GetParams(ctx),
		NextDiscrepancyID: k.GetNextDiscrepancyID(ctx),
		NextAuditEscrowID: k.GetNextAuditEscrowID(ctx),
		NextGraceRecordID: k.GetNextGraceRecordID(ctx),
	}

	k.WithAuditors(ctx, func(record vtypes.AuditorRecord) bool {
		state.Auditors = append(state.Auditors, record)
		return false
	})
	k.WithProviderBonds(ctx, func(record vtypes.ProviderBondRecord) bool {
		state.ProviderBonds = append(state.ProviderBonds, record)
		return false
	})
	k.WithProviderSnapshots(ctx, func(record vtypes.ProviderSnapshotRecord) bool {
		state.ProviderSnapshots = append(state.ProviderSnapshots, record)
		return false
	})
	k.WithDiscrepancies(ctx, vtypes.DiscrepancyStatusUnspecified, func(record vtypes.DiscrepancyEvent) bool {
		state.Discrepancies = append(state.Discrepancies, record)
		return false
	})
	k.WithAuditEscrows(ctx, func(record vtypes.AuditEscrowRecord) bool {
		state.AuditEscrows = append(state.AuditEscrows, record)
		return false
	})
	k.WithVerificationGraces(ctx, func(record vtypes.ProviderVerificationGraceRecord) bool {
		state.VerificationGraces = append(state.VerificationGraces, record)
		return false
	})
	k.WithAttestations(ctx, func(record vtypes.AttestationRecord) bool {
		state.Attestations = append(state.Attestations, record)
		return false
	})

	return state
}

func DefaultGenesisState() *vtypes.GenesisState {
	return &vtypes.GenesisState{
		Params:            keeper.DefaultParams(),
		NextDiscrepancyID: 1,
		NextAuditEscrowID: 1,
		NextGraceRecordID: 1,
	}
}

func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *vtypes.GenesisState {
	var genesisState vtypes.GenesisState

	if appState[moduletypes.ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[moduletypes.ModuleName], &genesisState)
	}

	return &genesisState
}

func validateProviderAuditor(provider, auditor string) error {
	if _, err := sdk.AccAddressFromBech32(provider); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrap("verification: invalid provider address")
	}
	if _, err := sdk.AccAddressFromBech32(auditor); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrap("verification: invalid auditor address")
	}
	if provider == auditor {
		return moduletypes.ErrSelfAttestation
	}

	return nil
}

func validateTier(tier vtypes.VerificationTier) error {
	switch tier {
	case vtypes.TierUnspecified, vtypes.TierIdentified, vtypes.TierVerified, vtypes.TierEstablished, vtypes.TierTrusted:
		return nil
	default:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "verification: invalid tier %d", tier)
	}
}
