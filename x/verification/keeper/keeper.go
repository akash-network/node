package keeper

import (
	"context"
	"time"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
)

type Keeper interface {
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	NewQuerier() vtypes.QueryServer
	GetAuthority() string
	EndBlocker(context.Context) error
	Settle(SettlementInput) (SettlementResult, error)
	GetParams(sdk.Context) vtypes.Params
	SetParams(sdk.Context, vtypes.Params)
	GetAuditor(sdk.Context, sdk.AccAddress) (vtypes.AuditorRecord, bool)
	SetAuditor(sdk.Context, vtypes.AuditorRecord) error
	WithAuditors(sdk.Context, func(vtypes.AuditorRecord) bool)
	GetAttestation(sdk.Context, sdk.AccAddress, sdk.AccAddress) (vtypes.AttestationRecord, bool)
	SetAttestation(sdk.Context, vtypes.AttestationRecord) error
	WithAttestations(sdk.Context, func(vtypes.AttestationRecord) bool)
	WithProviderAttestations(sdk.Context, sdk.AccAddress, vtypes.AttestationStatus, func(vtypes.AttestationRecord) bool)
	WithAuditorAttestations(sdk.Context, sdk.AccAddress, func(vtypes.AttestationRecord) bool)
	GetDiscrepancy(sdk.Context, uint64) (vtypes.DiscrepancyEvent, bool)
	SetDiscrepancy(sdk.Context, vtypes.DiscrepancyEvent)
	WithDiscrepancies(sdk.Context, vtypes.DiscrepancyStatus, func(vtypes.DiscrepancyEvent) bool)
	GetProviderBond(sdk.Context, sdk.AccAddress) (vtypes.ProviderBondRecord, bool)
	SetProviderBond(sdk.Context, vtypes.ProviderBondRecord) error
	WithProviderBonds(sdk.Context, func(vtypes.ProviderBondRecord) bool)
	GetProviderSnapshot(sdk.Context, sdk.AccAddress) (vtypes.ProviderSnapshotRecord, bool)
	SetProviderSnapshot(sdk.Context, vtypes.ProviderSnapshotRecord) error
	WithProviderSnapshots(sdk.Context, func(vtypes.ProviderSnapshotRecord) bool)
	GetAuditEscrow(sdk.Context, uint64) (vtypes.AuditEscrowRecord, bool)
	SetAuditEscrow(sdk.Context, vtypes.AuditEscrowRecord) error
	WithAuditEscrows(sdk.Context, func(vtypes.AuditEscrowRecord) bool)
	WithProviderAuditEscrows(sdk.Context, sdk.AccAddress, vtypes.AuditEscrowStatus, func(vtypes.AuditEscrowRecord) bool)
	GetProviderVerificationGrace(sdk.Context, sdk.AccAddress) (vtypes.ProviderVerificationGraceRecord, bool)
	SetProviderVerificationGrace(sdk.Context, vtypes.ProviderVerificationGraceRecord) error
	WithVerificationGraces(sdk.Context, func(vtypes.ProviderVerificationGraceRecord) bool)
	GetNextDiscrepancyID(sdk.Context) uint64
	SetNextDiscrepancyID(sdk.Context, uint64)
	GetNextAuditEscrowID(sdk.Context) uint64
	SetNextAuditEscrowID(sdk.Context, uint64)
	NextAuditEscrowID(sdk.Context) uint64
	GetNextGraceRecordID(sdk.Context) uint64
	SetNextGraceRecordID(sdk.Context, uint64)
	RegisterAuditor(sdk.Context, string, sdk.AccAddress, vtypes.VerificationTier, []byte) error
	PostAuditorBond(sdk.Context, sdk.AccAddress, sdk.Coin) error
	PostProviderBond(sdk.Context, sdk.AccAddress, sdk.Coin) error
	PostSnapshotHash(sdk.Context, sdk.AccAddress, []byte, vtypes.ResourceSummary, time.Time) error
	OpenAuditEscrow(sdk.Context, sdk.AccAddress, vtypes.VerificationTier, []vtypes.CapabilityFlag, sdk.Coin, sdk.Coin, time.Time, []byte) (uint64, error)
	CancelAuditEscrow(sdk.Context, sdk.AccAddress, uint64) error
	SettleAuditEscrow(sdk.Context, string, uint64, vtypes.AuditEscrowSettlementReason, vtypes.FaultAttribution, []byte) error
	SubmitAttestation(sdk.Context, sdk.AccAddress, sdk.AccAddress, vtypes.VerificationTier, []vtypes.CapabilityFlag, []byte, sdk.Coin, sdk.Coin, uint64) error
	BidFilter(sdk.Context, sdk.AccAddress, *vtypes.VerificationRequirement) error
}

type keeper struct {
	cdc       codec.BinaryCodec
	skey      storetypes.StoreKey
	authority string
	bank      BankKeeper
	provider  ProviderKeeper
}

func NewKeeper(cdc codec.BinaryCodec, skey storetypes.StoreKey, opts ...Option) Keeper {
	k := &keeper{
		cdc:  cdc,
		skey: skey,
	}
	for _, opt := range opts {
		opt(k)
	}
	return k
}

func (k *keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

func (k *keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

func (k *keeper) Logger(sctx sdk.Context) log.Logger {
	return sctx.Logger().With("module", "x/"+moduletypes.ModuleName)
}

func (k *keeper) NewQuerier() vtypes.QueryServer {
	return &Querier{Keeper: k}
}

func (k *keeper) GetAuthority() string {
	return k.authority
}

func (k *keeper) Settle(input SettlementInput) (SettlementResult, error) {
	return Settle(input)
}
