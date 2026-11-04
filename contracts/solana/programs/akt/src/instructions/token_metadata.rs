//! `set_token_metadata`: the AKT mint's Metaplex metadata.

use super::upgrade_authority::*;
use crate::{constants::*, errors::AktError, events::TokenMetadataSet, state::LaunchConfig};
use anchor_lang::prelude::*;
use anchor_spl::{
    metadata::{
        self, mpl_token_metadata::types::DataV2, CreateMetadataAccountsV3, Metadata,
        MetadataAccount, UpdateMetadataAccountsV2,
    },
    token::Mint,
};

#[derive(Accounts)]
pub struct SetTokenMetadata<'info> {
    /// Pays rent when the metadata account is created.
    pub admin: UpgradeAuthority<'info>,
    #[account(seeds = [CONFIG_SEED], bump = config.bump, has_one = mint)]
    pub config: Account<'info, LaunchConfig>,
    #[account(seeds = [MINT_SEED], bump, mint::decimals = AKT_DECIMALS, mint::authority = mint_authority)]
    pub mint: Account<'info, Mint>,
    /// CHECK: The `["mint_authority"]` PDA with its stored bump. It signs the
    /// Metaplex CPI as mint authority and is the metadata update authority.
    #[account(seeds = [MINT_AUTHORITY_SEED], bump = config.mint_authority_bump)]
    pub mint_authority: UncheckedAccount<'info>,
    /// CHECK: The mint's metadata PDA under `metadata_program`. If it exists,
    /// the handler checks its owner, mint and update authority first.
    #[account(
        mut,
        seeds = [METADATA_SEED, metadata_program.key().as_ref(), mint.key().as_ref()],
        bump,
        seeds::program = metadata_program.key()
    )]
    pub metadata: UncheckedAccount<'info>,
    pub metadata_program: Program<'info, Metadata>,
    pub system_program: Program<'info, System>,
    /// Part of Metaplex's `CreateMetadataAccountsV3` account list.
    pub rent: Sysvar<'info, Rent>,
}

/// Creates the mint's metadata, or rewrites its name, symbol and URI if it
/// exists: name `TOKEN_NAME`, symbol `TOKEN_SYMBOL`, no creators, collection,
/// uses or royalties, mutable, with the mint-authority PDA as update authority.
/// Metaplex lets only the mint authority create metadata, so no one else can
/// create it first. Supply and token authorities never change.
pub(crate) fn set_token_metadata(ctx: Context<SetTokenMetadata>, uri: String) -> Result<()> {
    // Operators check the URL and its JSON off-chain. The program bounds the
    // field and rejects spaces and control characters.
    require!(
        uri.starts_with(METADATA_URI_SCHEME)
            && uri.len() > METADATA_URI_SCHEME.len()
            && uri.len() <= MAX_METADATA_URI_LEN
            && uri.bytes().all(|byte| byte.is_ascii_graphic()),
        AktError::InvalidMetadataUri
    );
    let data = DataV2 {
        name: TOKEN_NAME.into(),
        symbol: TOKEN_SYMBOL.into(),
        uri: uri.clone(),
        seller_fee_basis_points: 0,
        creators: None,
        collection: None,
        uses: None,
    };
    let bump = [ctx.accounts.config.mint_authority_bump];
    let seeds: &[&[u8]] = &[MINT_AUTHORITY_SEED, &bump];
    let signers = &[seeds];
    if ctx.accounts.metadata.data_is_empty() {
        metadata::create_metadata_accounts_v3(
            CpiContext::new_with_signer(
                ctx.accounts.metadata_program.to_account_info(),
                CreateMetadataAccountsV3 {
                    metadata: ctx.accounts.metadata.to_account_info(),
                    mint: ctx.accounts.mint.to_account_info(),
                    mint_authority: ctx.accounts.mint_authority.to_account_info(),
                    payer: ctx.accounts.admin.authority.to_account_info(),
                    update_authority: ctx.accounts.mint_authority.to_account_info(),
                    system_program: ctx.accounts.system_program.to_account_info(),
                    rent: ctx.accounts.rent.to_account_info(),
                },
                signers,
            ),
            data,
            true, // is_mutable
            true, // update_authority_is_signer
            None, // collection_details
        )?;
    } else {
        require_keys_eq!(
            *ctx.accounts.metadata.owner,
            metadata::ID,
            AktError::InvalidMetadata
        );
        let existing = MetadataAccount::try_deserialize(
            &mut ctx.accounts.metadata.try_borrow_data()?.as_ref(),
        )?;
        require!(
            existing.mint == ctx.accounts.mint.key()
                && existing.update_authority == ctx.accounts.mint_authority.key(),
            AktError::InvalidMetadata
        );
        metadata::update_metadata_accounts_v2(
            CpiContext::new_with_signer(
                ctx.accounts.metadata_program.to_account_info(),
                UpdateMetadataAccountsV2 {
                    metadata: ctx.accounts.metadata.to_account_info(),
                    update_authority: ctx.accounts.mint_authority.to_account_info(),
                },
                signers,
            ),
            None, // new_update_authority: unchanged
            Some(data),
            None, // primary_sale_happened: unchanged
            None, // is_mutable: unchanged
        )?;
    }
    emit!(TokenMetadataSet {
        mint: ctx.accounts.mint.key(),
        metadata: ctx.accounts.metadata.key(),
        uri,
    });
    Ok(())
}
