//! Proof buffers: ICS23 proofs are larger than one transaction, so submitters
//! upload them in chunks before calling `mint_transfer` or `void_transfer`.

use crate::{constants::*, errors::AktError, state::ProofBuffer};
use anchor_lang::prelude::*;

#[derive(Accounts)]
#[instruction(nonce: [u8; 32], expected_len: u32)]
pub struct CreateProofBuffer<'info> {
    /// Owns the buffer and pays its rent.
    #[account(mut)]
    pub submitter: Signer<'info>,
    #[account(init, payer = submitter, space = ProofBuffer::initial_space(expected_len)?,
        seeds = [PROOF_SEED, submitter.key().as_ref(), &nonce], bump)]
    pub proof_buffer: Box<Account<'info, ProofBuffer>>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct GrowProofBuffer<'info> {
    #[account(mut)]
    pub submitter: Signer<'info>,
    #[account(mut, has_one = submitter,
        seeds = [PROOF_SEED, submitter.key().as_ref(), proof_buffer.nonce.as_ref()], bump,
        realloc = proof_buffer.grown_space(proof_buffer.to_account_info().data_len()),
        realloc::payer = submitter, realloc::zero = true)]
    pub proof_buffer: Box<Account<'info, ProofBuffer>>,
    pub system_program: Program<'info, System>,
}

/// Accounts of `append_proof`.
#[derive(Accounts)]
pub struct WriteProofBuffer<'info> {
    pub submitter: Signer<'info>,
    #[account(mut, has_one = submitter,
        seeds = [PROOF_SEED, submitter.key().as_ref(), proof_buffer.nonce.as_ref()], bump)]
    pub proof_buffer: Box<Account<'info, ProofBuffer>>,
}

#[derive(Accounts)]
pub struct CloseProofBuffer<'info> {
    /// Receives the rent.
    #[account(mut)]
    pub submitter: Signer<'info>,
    #[account(mut, has_one = submitter, close = submitter,
        seeds = [PROOF_SEED, submitter.key().as_ref(), proof_buffer.nonce.as_ref()], bump)]
    pub proof_buffer: Box<Account<'info, ProofBuffer>>,
}

/// Creates an empty buffer for a proof of `expected_len` bytes.
pub(crate) fn create_proof_buffer(
    ctx: Context<CreateProofBuffer>,
    nonce: [u8; 32],
    expected_len: u32,
) -> Result<()> {
    ctx.accounts.proof_buffer.set_inner(ProofBuffer {
        submitter: ctx.accounts.submitter.key(),
        nonce,
        expected_len,
        data: Vec::new(),
    });
    Ok(())
}

pub(crate) fn grow_proof_buffer(_ctx: Context<GrowProofBuffer>) -> Result<()> {
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
        !chunk.is_empty() && chunk.len() <= MAX_PROOF_CHUNK_BYTES && chunk.len() <= remaining,
        AktError::InvalidProofLength
    );
    require!(
        ProofBuffer::HEADER_LEN + written + chunk.len() <= buffer.to_account_info().data_len(),
        AktError::ProofBufferTooSmall
    );
    buffer.data.extend_from_slice(&chunk);
    Ok(())
}

/// Closes the buffer; Anchor's `close` constraint returns the rent to the
/// submitter. Transfer statuses are separate accounts and are never closed.
pub(crate) fn close_proof_buffer(_ctx: Context<CloseProofBuffer>) -> Result<()> {
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::BTreeSet;

    #[test]
    fn only_the_submitter_can_write_or_close_the_canonical_buffer() {
        let submitter = Pubkey::new_unique();
        let nonce = [9; 32];
        let address =
            Pubkey::find_program_address(&[PROOF_SEED, submitter.as_ref(), &nonce], &crate::ID).0;
        for (signer, is_signer, key, valid) in [
            (submitter, true, address, true),
            (Pubkey::new_unique(), true, address, false),
            (submitter, false, address, false),
            (submitter, true, Pubkey::new_unique(), false),
        ] {
            let mut signer_lamports = 1;
            let mut signer_data = [];
            let mut buffer_lamports = 1;
            let mut buffer_data = Vec::new();
            ProofBuffer {
                submitter,
                nonce,
                expected_len: 3,
                data: vec![],
            }
            .try_serialize(&mut buffer_data)
            .unwrap();
            buffer_data.resize(8 + ProofBuffer::INIT_SPACE, 0);
            let accounts = [
                AccountInfo::new(
                    &signer,
                    is_signer,
                    true,
                    &mut signer_lamports,
                    &mut signer_data,
                    &anchor_lang::system_program::ID,
                    false,
                    0,
                ),
                AccountInfo::new(
                    &key,
                    false,
                    true,
                    &mut buffer_lamports,
                    &mut buffer_data,
                    &crate::ID,
                    false,
                    0,
                ),
            ];
            let mut bumps = WriteProofBufferBumps::default();
            let mut remaining = accounts.as_slice();
            let write = WriteProofBuffer::try_accounts(
                &crate::ID,
                &mut remaining,
                &[],
                &mut bumps,
                &mut BTreeSet::new(),
            );
            assert_eq!(write.is_ok(), valid);
            if let Ok(mut write) = write {
                let mut append = |offset, chunk| {
                    append_proof(
                        Context::new(
                            &crate::ID,
                            &mut write,
                            &[],
                            WriteProofBufferBumps {
                                proof_buffer: bumps.proof_buffer,
                            },
                        ),
                        offset,
                        chunk,
                    )
                };
                assert!(append(0, vec![1, 2]).is_ok());
                assert_eq!(
                    append(0, vec![3]).unwrap_err(),
                    AktError::InvalidProofOffset.into()
                );
                assert_eq!(
                    append(2, vec![]).unwrap_err(),
                    AktError::InvalidProofLength.into()
                );
                assert_eq!(
                    append(2, vec![3, 4]).unwrap_err(),
                    AktError::InvalidProofLength.into()
                );
                assert!(append(2, vec![3]).is_ok());
                assert_eq!(
                    append(3, vec![4]).unwrap_err(),
                    AktError::InvalidProofLength.into()
                );
                assert!(write.proof_buffer.is_complete());
                assert_eq!(write.proof_buffer.data, [1, 2, 3]);
            }
            let mut remaining = accounts.as_slice();
            let close = CloseProofBuffer::try_accounts(
                &crate::ID,
                &mut remaining,
                &[],
                &mut CloseProofBufferBumps::default(),
                &mut BTreeSet::new(),
            );
            assert_eq!(close.is_ok(), valid);
        }
    }
}
