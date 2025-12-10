package keeper

import (
	errorsmod "cosmossdk.io/errors"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmvmtypes "github.com/CosmWasm/wasmvm/v3/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	wv1 "pkg.akt.dev/go/node/wasm/v1"
)

// FilterMessenger wraps the default messenger with Phase 1 restrictions
type FilterMessenger struct {
	k    *keeper
	next wasmkeeper.Messenger
}

// NewMsgFilterDecorator returns the message filter decorator
func (k *keeper) NewMsgFilterDecorator() func(wasmkeeper.Messenger) wasmkeeper.Messenger {
	return func(next wasmkeeper.Messenger) wasmkeeper.Messenger {
		return &FilterMessenger{
			k:    k,
			next: next,
		}
	}
}

// DispatchMsg applies Phase 1 filtering before dispatching
func (m *FilterMessenger) DispatchMsg(
	ctx sdk.Context,
	contractAddr sdk.AccAddress,
	contractIBCPortID string,
	msg wasmvmtypes.CosmosMsg,
) (events []sdk.Event, data [][]byte, msgResponses [][]*codectypes.Any, err error) {
	// Apply Phase 1 restrictions
	if err := m.k.FilterMessage(ctx, contractAddr, msg); err != nil {
		// Emit event for monitoring
		_ = ctx.EventManager().EmitTypedEvent(
			&wv1.EventMsgBlocked{
				ContractAddress: contractAddr.String(),
				MsgType:         getMessageType(msg),
				Reason:          err.Error(),
			},
		)

		ctx.Logger().Info("Phase 1: Message blocked",
			"contract", contractAddr.String(),
			"type", getMessageType(msg),
			"reason", err.Error(),
		)

		return nil, nil, nil, err
	}

	// Pass to wrapped messenger
	return m.next.DispatchMsg(ctx, contractAddr, contractIBCPortID, msg)
}

// FilterMessage applies Phase 1 filtering rules
func (k *keeper) FilterMessage(sctx sdk.Context, contractAddr sdk.AccAddress, msg wasmvmtypes.CosmosMsg) error {
	// ALLOW Bank messages (with restrictions)
	if msg.Bank != nil {
		return k.filterBankMessage(sctx, msg.Bank)
	}

	// BLOCK Staking messages
	if msg.Staking != nil {
		return errorsmod.Wrap(
			sdkerrors.ErrUnauthorized,
			"Staking operations not allowed",
		)
	}

	// BLOCK Distribution messages
	if msg.Distribution != nil {
		return errorsmod.Wrap(
			sdkerrors.ErrUnauthorized,
			"Distribution operations not allowed",
		)
	}

	// BLOCK Governance messages
	if msg.Gov != nil {
		return errorsmod.Wrap(
			sdkerrors.ErrUnauthorized,
			"Governance operations not allowed",
		)
	}

	// BLOCK IBC messages
	if msg.IBC != nil {
		return errorsmod.Wrap(
			sdkerrors.ErrUnauthorized,
			"IBC messages not allowed",
		)
	}

	if msg.IBC2 != nil {
		return errorsmod.Wrap(
			sdkerrors.ErrUnauthorized,
			"IBC2 messages not allowed",
		)
	}

	// BLOCK Custom messages (no Akash bindings)
	if msg.Custom != nil {
		return errorsmod.Wrap(
			sdkerrors.ErrUnauthorized,
			"Custom messages not allowed",
		)
	}

	// ALLOW specific Any messages from authorized contracts
	if msg.Any != nil {
		return k.filterAnyMessage(sctx, contractAddr, msg.Any)
	}

	// ALLOW Wasm messages (contract-to-contract calls)
	if msg.Wasm != nil {
		// Wasm execute/instantiate allowed
		return nil
	}

	// BLOCK unknown/unhandled message types
	return errorsmod.Wrap(
		sdkerrors.ErrUnauthorized,
		"Unknown message type not allowed",
	)
}

// filterBankMessage applies restrictions to bank operations
func (k *keeper) filterBankMessage(sctx sdk.Context, msg *wasmvmtypes.BankMsg) error {
	// Allow send with restrictions
	if msg.Send != nil {
		params := k.GetParams(sctx)

		// Block transfers to critical addresses
		for _, addr := range params.BlockedAddresses {
			if addr == msg.Send.ToAddress {
				return errorsmod.Wrapf(
					sdkerrors.ErrUnauthorized,
					"Transfers to %s blocked (critical address)",
					msg.Send.ToAddress,
				)
			}
		}

		// Transfers to regular addresses allowed
		return nil
	}

	// Deny burns
	if msg.Burn != nil {
		return errorsmod.Wrapf(
			sdkerrors.ErrUnauthorized,
			"Burn is not allowed",
		)
	}

	return nil
}

// filterAnyMessage applies restrictions to Any (protobuf) messages
// Only MsgAddPriceEntry from authorized oracle sources is allowed
func (k *keeper) filterAnyMessage(sctx sdk.Context, contractAddr sdk.AccAddress, msg *wasmvmtypes.AnyMsg) error {
	// Only allow MsgAddPriceEntry from oracle module
	if msg.TypeURL != "/akash.oracle.v1.MsgAddPriceEntry" {
		return errorsmod.Wrapf(
			sdkerrors.ErrUnauthorized,
			"Any message type %s not allowed",
			msg.TypeURL,
		)
	}

	return nil
}

// getMessageType returns a human-readable message type
func getMessageType(msg wasmvmtypes.CosmosMsg) string {
	if msg.Bank != nil {
		if msg.Bank.Send != nil {
			return "bank.send"
		}
		if msg.Bank.Burn != nil {
			return "bank.burn"
		}
		return "bank.unknown"
	}
	if msg.Staking != nil {
		return "staking"
	}
	if msg.Distribution != nil {
		return "distribution"
	}
	if msg.IBC != nil {
		return "ibc"
	}
	if msg.IBC2 != nil {
		return "ibc2"
	}
	if msg.Wasm != nil {
		return "wasm"
	}
	if msg.Gov != nil {
		return "gov"
	}
	if msg.Custom != nil {
		return "custom"
	}

	if msg.Any != nil {
		return msg.Any.TypeURL
	}

	return "unknown"
}
