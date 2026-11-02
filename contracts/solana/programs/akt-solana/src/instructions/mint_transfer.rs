//! `mint_transfer`: resolves a proven x/sink record by minting to its
//! recipient, or by voiding it when the recipient cannot receive AKT.

use super::resolution::{require_top_level_invocation, resolve};
use crate::{
    constants::*,
    errors::AktError,
    light_client::{self, MembershipAccounts},
    state::{BridgeConfig, LaunchConfig, ProofBuffer, TransferOutcome, TransferStatus, VoidReason},
    wire::TransferRecord,
};
use anchor_lang::{prelude::*, solana_program::program_pack::Pack, system_program};
use anchor_spl::{
    associated_token::{self, AssociatedToken},
    token::{
        self,
        spl_token::state::{Account as TokenState, AccountState},
        Mint, MintTo, Token,
    },
    token_2022,
};
use solana_curve25519::edwards::{validate_edwards, PodEdwardsPoint};

#[derive(Accounts)]
#[instruction(record: TransferRecord)]
pub struct MintTransfer<'info> {
    /// Pays rent for the status account and, on a mint, the recipient's
    /// associated token account if it does not exist.
    #[account(mut)]
    pub payer: Signer<'info>,
    #[account(seeds = [CONFIG_SEED], bump = config.bump,
        constraint = config.bridge_enabled @ AktError::BridgeDisabled)]
    pub config: Account<'info, LaunchConfig>,
    /// Pinned client and policy. A mint updates the day's usage.
    #[account(mut, seeds = [BRIDGE_SEED], bump)]
    pub bridge: Account<'info, BridgeConfig>,
    /// A complete proof of the transfer key of `record.id`, from any
    /// submitter's buffer.
    pub proof_buffer: Box<Account<'info, ProofBuffer>>,
    /// Terminal status. `init` fails if the record already has one.
    #[account(init, payer = payer, space = 8 + TransferStatus::INIT_SPACE,
        seeds = [TRANSFER_SEED, &record.id.to_le_bytes()], bump)]
    pub status: Account<'info, TransferStatus>,
    /// CHECK: Must be `bridge.light_client_program` and executable.
    #[account(address = bridge.light_client_program, executable)]
    pub light_client_program: UncheckedAccount<'info>,
    /// CHECK: `light_client::verify_membership` checks its owner, PDA, type,
    /// chain ID, frozen height and trusting period.
    pub client_state: UncheckedAccount<'info>,
    /// CHECK: `light_client::verify_membership` checks its owner, PDA and height
    /// (the client's latest) and that it is inside the trusting period.
    pub latest_consensus: UncheckedAccount<'info>,
    /// CHECK: `light_client::verify_membership` checks its owner, PDA and height
    /// (`proof_height`); ICS07 verifies the proof against its root.
    pub proof_consensus: UncheckedAccount<'info>,
    #[account(mut, seeds = [MINT_SEED], bump, address = config.mint,
        mint::authority = mint_authority, mint::token_program = token_program)]
    pub mint: Account<'info, Mint>,
    /// CHECK: The `["mint_authority"]` PDA with its stored bump. It holds no
    /// data and signs the mint CPI.
    #[account(seeds = [MINT_AUTHORITY_SEED], bump = config.mint_authority_bump)]
    pub mint_authority: UncheckedAccount<'info>,
    /// CHECK: Must be `record.recipient`. After the proof, `accepts_recipient`
    /// checks its owner, executable flag and curve point.
    #[account(address = record.recipient)]
    pub recipient: UncheckedAccount<'info>,
    /// CHECK: Must be the recipient's legacy-token associated token account for
    /// the AKT mint. `ata_usable` checks any existing contents before a mint.
    #[account(mut, address = associated_token::get_associated_token_address(&record.recipient, &mint.key()))]
    pub recipient_token_account: UncheckedAccount<'info>,
    pub token_program: Program<'info, Token>,
    pub associated_token_program: Program<'info, AssociatedToken>,
    pub system_program: Program<'info, System>,
}

/// Verifies that the transfer key of `record.id` holds `record` at
/// `proof_height`, then mints `record.amount` to the recipient's associated
/// token account if `accepts_recipient`, or records
/// `Voided / InvalidRecipient`. Either way the status is permanent. Caps apply
/// only to mints, and a cap failure reverts the instruction, so the record
/// stays pending and can be retried.
pub(crate) fn mint_transfer(
    ctx: Context<MintTransfer>,
    record: TransferRecord,
    proof_height: u64,
) -> Result<()> {
    require_top_level_invocation()?;
    record.validate(proof_height)?;
    let accounts = ctx.accounts;
    light_client::verify_membership(
        MembershipAccounts {
            program: &accounts.light_client_program,
            client: &accounts.client_state,
            latest: &accounts.latest_consensus,
            consensus: &accounts.proof_consensus,
        },
        &accounts.bridge,
        &accounts.proof_buffer,
        proof_height,
        record.key(),
        record.try_to_vec()?,
    )?;

    let outcome = if accepts_recipient(accounts)? {
        issue(accounts, record.amount)?;
        TransferOutcome::Minted {
            amount: record.amount,
            recipient: record.recipient,
        }
    } else {
        TransferOutcome::Voided {
            reason: VoidReason::InvalidRecipient,
        }
    };
    resolve(&mut accounts.status, record.id, proof_height, outcome)
}

/// Whether the proven recipient may receive AKT.
///
/// Accepted: a System-owned key on the ed25519 curve (a wallet, which need not
/// exist yet), or an allowlisted vault. Refused even when allowlisted:
/// executables, the zero key, the mint-authority PDA and accounts owned by
/// either token program. The recipient's associated token account must also
/// be usable. The curve check is a format check; it does not prove that
/// anyone holds the key.
fn accepts_recipient(accounts: &MintTransfer) -> Result<bool> {
    let recipient = &accounts.recipient;
    let key = recipient.key();
    let refused = recipient.executable
        || key == Pubkey::default()
        || key == accounts.mint_authority.key()
        || recipient.owner == &token::ID
        || recipient.owner == &token_2022::ID;
    // solana-pubkey's is_on_curve is unimplemented on SBF; validate_edwards
    // uses the curve25519 point-validation syscall.
    let wallet = recipient.owner == &system_program::ID
        && validate_edwards(&PodEdwardsPoint(key.to_bytes()));
    let vault = accounts.bridge.policy.vaults.contains(&key);
    let ata = &accounts.recipient_token_account;
    let ata_ok = ata_usable(
        ata.owner,
        &ata.try_borrow_data()?,
        &accounts.mint.key(),
        &key,
    );
    Ok(!refused && (wallet || vault) && ata_ok)
}

/// Charges `amount` to the day's caps, creates the recipient's associated
/// token account if it does not exist and mints `amount` to it. A cap failure
/// returns an error, which reverts the instruction.
fn issue(accounts: &mut MintTransfer, amount: u64) -> Result<()> {
    let now = u64::try_from(Clock::get()?.unix_timestamp).map_err(|_| AktError::InvalidClock)?;
    let day = now / SECONDS_PER_DAY;
    let total = accounts.bridge.next_mint_total(day, amount)?;

    associated_token::create_idempotent(CpiContext::new(
        accounts.associated_token_program.to_account_info(),
        associated_token::Create {
            payer: accounts.payer.to_account_info(),
            associated_token: accounts.recipient_token_account.to_account_info(),
            authority: accounts.recipient.to_account_info(),
            mint: accounts.mint.to_account_info(),
            system_program: accounts.system_program.to_account_info(),
            token_program: accounts.token_program.to_account_info(),
        },
    ))?;
    let bump = [accounts.config.mint_authority_bump];
    let seeds: &[&[u8]] = &[MINT_AUTHORITY_SEED, &bump];
    token::mint_to(
        CpiContext::new_with_signer(
            accounts.token_program.to_account_info(),
            MintTo {
                mint: accounts.mint.to_account_info(),
                to: accounts.recipient_token_account.to_account_info(),
                authority: accounts.mint_authority.to_account_info(),
            },
            &[seeds],
        ),
        amount,
    )?;
    accounts.bridge.mint_day = day;
    accounts.bridge.minted_today = total;
    Ok(())
}

/// Whether the recipient's associated token account can take the mint: absent
/// (System-owned and empty; `create_idempotent` creates it), or an initialized
/// legacy-token account for the AKT mint owned by the recipient. Legacy SPL
/// Token lets a holder reassign its account's owner, after which
/// `create_idempotent` fails on every attempt. Refusing such an account voids
/// the transfer instead of leaving it pending forever.
fn ata_usable(program: &Pubkey, data: &[u8], mint: &Pubkey, wallet: &Pubkey) -> bool {
    if program == &system_program::ID {
        return data.is_empty();
    }
    program == &token::ID
        && TokenState::unpack(data).is_ok_and(|account| {
            account.mint == *mint
                && account.owner == *wallet
                && account.state == AccountState::Initialized
        })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn existing_ata_must_be_an_initialized_akt_account_of_the_recipient() {
        let (mint, wallet) = (Pubkey::new_unique(), Pubkey::new_unique());
        let system = system_program::ID;
        let pack = |mint, owner, state| {
            let mut data = vec![0; TokenState::LEN];
            TokenState {
                mint,
                owner,
                state,
                ..TokenState::default()
            }
            .pack_into_slice(&mut data);
            data
        };
        let valid = pack(mint, wallet, AccountState::Initialized);
        assert!(ata_usable(&system, &[], &mint, &wallet));
        assert!(!ata_usable(&system, &[0], &mint, &wallet));
        assert!(ata_usable(&token::ID, &valid, &mint, &wallet));
        assert!(!ata_usable(&token_2022::ID, &valid, &mint, &wallet));
        assert!(!ata_usable(&token::ID, &valid[1..], &mint, &wallet));
        let other = Pubkey::new_unique();
        for data in [
            pack(mint, other, AccountState::Initialized),
            pack(other, wallet, AccountState::Initialized),
            pack(mint, wallet, AccountState::Frozen),
            pack(mint, wallet, AccountState::Uninitialized),
        ] {
            assert!(!ata_usable(&token::ID, &data, &mint, &wallet));
        }
    }
}
