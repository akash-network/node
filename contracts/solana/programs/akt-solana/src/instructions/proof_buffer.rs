//! Proof buffers: ICS23 proofs are larger than one transaction, so submitters
//! upload them in chunks before calling `mint_transfer` or `void_transfer`.

use crate::{constants::*, errors::AktError, state::ProofBuffer};
use anchor_lang::prelude::*;

#[derive(Accounts)]
#[instruction(nonce: u64)]
pub struct CreateProofBuffer<'info> {
    /// Owns the buffer and pays its rent.
    #[account(mut)]
    pub submitter: Signer<'info>,
    /// Sized for `MAX_PROOF_BYTES` whatever the declared length.
    #[account(init, payer = submitter, space = 8 + ProofBuffer::INIT_SPACE,
        seeds = [PROOF_SEED, submitter.key().as_ref(), &nonce.to_le_bytes()], bump)]
    pub proof_buffer: Box<Account<'info, ProofBuffer>>,
    pub system_program: Program<'info, System>,
}

/// Accounts of `append_proof`.
#[derive(Accounts)]
pub struct WriteProofBuffer<'info> {
    pub submitter: Signer<'info>,
    #[account(mut, has_one = submitter)]
    pub proof_buffer: Box<Account<'info, ProofBuffer>>,
}

#[derive(Accounts)]
pub struct CloseProofBuffer<'info> {
    /// Receives the rent.
    #[account(mut)]
    pub submitter: Signer<'info>,
    #[account(mut, has_one = submitter, close = submitter)]
    pub proof_buffer: Box<Account<'info, ProofBuffer>>,
}

/// Creates an empty buffer for a proof of `expected_len` bytes.
pub(crate) fn create_proof_buffer(
    ctx: Context<CreateProofBuffer>,
    nonce: u64,
    expected_len: u32,
) -> Result<()> {
    require!(
        expected_len > 0 && expected_len as usize <= MAX_PROOF_BYTES,
        AktError::InvalidProofLength
    );
    ctx.accounts.proof_buffer.set_inner(ProofBuffer {
        submitter: ctx.accounts.submitter.key(),
        nonce,
        expected_len,
        data: Vec::new(),
    });
    Ok(())
}

/// Appends `chunk` at `offset`, which must equal the bytes already written.
/// The chunk must be nonempty and fit the declared length, so a complete
/// buffer accepts no more writes.
pub(crate) fn append_proof(
    ctx: Context<WriteProofBuffer>,
    offset: u32,
    chunk: Vec<u8>,
) -> Result<()> {
    let buffer = &mut ctx.accounts.proof_buffer;
    let written = buffer.data.len();
    require!(offset as usize == written, AktError::InvalidProofOffset);
    let remaining = (buffer.expected_len as usize).saturating_sub(written);
    require!(
        !chunk.is_empty() && chunk.len() <= remaining,
        AktError::InvalidProofLength
    );
    buffer.data.extend_from_slice(&chunk);
    Ok(())
}

/// Closes the buffer; Anchor's `close` constraint returns the rent to the
/// submitter. Transfer statuses are separate accounts and are never closed.
pub(crate) fn close_proof_buffer(_ctx: Context<CloseProofBuffer>) -> Result<()> {
    Ok(())
}
