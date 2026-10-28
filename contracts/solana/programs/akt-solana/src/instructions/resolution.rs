//! Rules shared by `mint_transfer` and `void_transfer`, the instructions that
//! resolve an x/sink record.

use crate::{
    constants::COMPUTE_BUDGET_PROGRAM_ID,
    errors::AktError,
    events::TransferResolved,
    state::{TransferOutcome, TransferStatus},
};
use anchor_lang::prelude::*;
use solana_instruction::{
    syscalls::{get_processed_sibling_instruction, get_stack_height},
    TRANSACTION_LEVEL_STACK_HEIGHT,
};

/// Requires a resolution to be a top-level instruction preceded only by
/// ComputeBudget instructions. Reporters confirm a resolution from its
/// `TransferResolved` event, so the event must be decodable and must fit in
/// the transaction's log:
/// - Anchor's event parser ignores events logged inside a CPI.
/// - Earlier instructions' logs count toward the 10 KB log limit and could
///   truncate the event. ComputeBudget instructions log only their invocation.
///
/// Later instructions are allowed: the event is already logged.
pub(crate) fn require_top_level_invocation() -> Result<()> {
    require_top_level(get_stack_height())?;
    // At the top level, processed siblings are the earlier top-level
    // instructions, most recent first.
    compute_budget_only(
        (0..)
            .map_while(get_processed_sibling_instruction)
            .map(|instruction| instruction.program_id),
    )
}

fn require_top_level(stack_height: usize) -> Result<()> {
    require!(
        stack_height == TRANSACTION_LEVEL_STACK_HEIGHT,
        AktError::CpiNotAllowed
    );
    Ok(())
}

fn compute_budget_only(mut programs: impl Iterator<Item = Pubkey>) -> Result<()> {
    require!(
        programs.all(|program| program == COMPUTE_BUDGET_PROGRAM_ID),
        AktError::PrecedingInstruction
    );
    Ok(())
}

/// Writes the record's terminal status and emits `TransferResolved`. The
/// instruction's `init` constraint created `status`, and fails if the record
/// already has one.
pub(crate) fn resolve(
    status: &mut Account<TransferStatus>,
    id: u64,
    proof_height: u64,
    outcome: TransferOutcome,
) -> Result<()> {
    status.set_inner(TransferStatus {
        id,
        proof_height,
        resolved_at_slot: Clock::get()?.slot,
        outcome: outcome.clone(),
    });
    emit!(TransferResolved {
        id,
        proof_height,
        outcome
    });
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_compute_budget_instructions_may_come_first() {
        let compute = COMPUTE_BUDGET_PROGRAM_ID;
        assert!(compute_budget_only([].into_iter()).is_ok());
        assert!(compute_budget_only([compute, compute].into_iter()).is_ok());
        for programs in [
            vec![anchor_lang::system_program::ID],
            vec![compute, crate::ID, compute],
        ] {
            assert_eq!(
                compute_budget_only(programs.into_iter()).unwrap_err(),
                AktError::PrecedingInstruction.into()
            );
        }
    }

    #[test]
    fn resolutions_require_top_level_invocation() {
        assert!(require_top_level(TRANSACTION_LEVEL_STACK_HEIGHT).is_ok());
        assert_eq!(
            require_top_level(TRANSACTION_LEVEL_STACK_HEIGHT + 1).unwrap_err(),
            AktError::CpiNotAllowed.into()
        );
    }
}
