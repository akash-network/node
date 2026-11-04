use super::*;

fn bridge_update(case: &mut Case, update: impl FnOnce(&mut BridgeConfig)) {
    let mut bridge: BridgeConfig = state(&case.accounts, case.bridge);
    update(&mut bridge);
    set(&mut case.accounts, case.bridge, &bridge);
}

fn replace_recipient(template: &Fixture, recipient: Pubkey) -> Fixture {
    let amount = template.amount.as_ref().unwrap().parse().unwrap();
    let mut fixture = reprove(
        template,
        bytes(&template.key),
        record_value(&recipient, amount).unwrap(),
    );
    fixture.recipient = Some(hex::encode(recipient.to_bytes()));
    fixture
}

pub(super) fn mint_checks(vm: &mut Mollusk, fixture: &Fixture) {
    for fault in [
        "amount-zero",
        "amount-mismatch",
        "recipient-mismatch",
        "transfer-cap",
        "daily-cap",
        "daily-overflow",
        "clock-regression",
        "mint-disabled",
        "mint-authority",
        "mint-address",
        "ata-address",
        "token-program",
        "insufficient-rent",
        "token-cpi-overflow",
    ] {
        let mut case = Case::new(fixture);
        match fault {
            "amount-zero" => case.instruction.data[76..84].fill(0),
            "amount-mismatch" => case.instruction.data[76..84].copy_from_slice(&2u64.to_le_bytes()),
            "recipient-mismatch" => case.instruction.data[44] ^= 1,
            "transfer-cap" => {
                let mut larger = reprove(
                    fixture,
                    bytes(&fixture.key),
                    record_value(&case.recipient, 2).unwrap(),
                );
                larger.amount = Some("2".into());
                case = Case::new(&larger);
                bridge_update(&mut case, |bridge| bridge.policy.per_transfer_max = 1);
            }
            "daily-cap" => bridge_update(&mut case, |bridge| {
                bridge.policy.per_transfer_max = 1;
                bridge.policy.daily_mint_cap = 1;
                bridge.minted_today = 1;
            }),
            "daily-overflow" => bridge_update(&mut case, |bridge| bridge.minted_today = u64::MAX),
            "clock-regression" => bridge_update(&mut case, |bridge| bridge.mint_day = 1),
            "mint-disabled" => {
                let mut config: LaunchConfig = state(&case.accounts, case.config);
                config.bridge_enabled = false;
                set(&mut case.accounts, case.config, &config);
            }
            "mint-authority" | "token-cpi-overflow" => {
                let account = get_mut(&mut case.accounts, case.mint);
                let mut mint = Mint::unpack(&account.data).unwrap();
                if fault == "mint-authority" {
                    mint.mint_authority = COption::Some(Pubkey::new_unique());
                } else {
                    mint.supply = u64::MAX;
                }
                Mint::pack(mint, &mut account.data).unwrap();
            }
            "mint-address" => case.replace_key(case.mint, Pubkey::new_unique()),
            "ata-address" => case.replace_key(case.ata, Pubkey::new_unique()),
            "token-program" => case.replace_key(token::ID, Pubkey::new_unique()),
            "insufficient-rent" => get_mut(&mut case.accounts, case.payer).lamports = 1,
            _ => unreachable!(),
        }
        reject(vm, fault, &case);
    }
    let wallet = (1..=255)
        .map(|value| Pubkey::new_from_array([value; 32]))
        .find(|key| key.is_on_curve())
        .unwrap();
    let wallet_fixture = replace_recipient(fixture, wallet);
    let mut case = Case::new(&wallet_fixture);
    bridge_update(&mut case, |bridge| bridge.policy.vaults.clear());
    let result = vm.process_instruction(&case.instruction, &case.accounts);
    assert!(result.raw_result.is_ok());
    assert_resolution(&result.resulting_accounts, &case, Some(1));
    println!("mint on_curve_wallet=true ata_created=true amount=1");

    for fault in [
        "executable",
        "token-owned",
        "token2022-owned",
        "off-curve-unlisted",
        "vault-nonsystem",
        "zero-recipient",
        "ata-frozen",
        "ata-other-owner",
        "ata-other-mint",
        "ata-malformed",
    ] {
        let off_curve = (1..=255)
            .map(|value| Pubkey::new_from_array([value; 32]))
            .find(|key| !key.is_on_curve())
            .unwrap();
        let recipient = if fault == "zero-recipient" {
            Pubkey::default()
        } else {
            off_curve
        };
        let mut case = Case::new(&replace_recipient(fixture, recipient));
        // The zero pubkey is the System Program itself, so it must be one account.
        if fault == "zero-recipient" {
            case.accounts
                .retain(|(key, account)| *key != recipient || account.executable);
        }
        match fault {
            "executable" => get_mut(&mut case.accounts, recipient).executable = true,
            "token-owned" => get_mut(&mut case.accounts, recipient).owner = token::ID,
            "token2022-owned" => {
                get_mut(&mut case.accounts, recipient).owner = anchor_spl::token_2022::ID
            }
            "off-curve-unlisted" => bridge_update(&mut case, |bridge| bridge.policy.vaults.clear()),
            "vault-nonsystem" => {
                get_mut(&mut case.accounts, recipient).owner = Pubkey::new_unique()
            }
            "zero-recipient" => {}
            _ => {
                let mut ata = TokenAccount {
                    mint: case.mint,
                    owner: recipient,
                    state: AccountState::Initialized,
                    ..TokenAccount::default()
                };
                match fault {
                    "ata-frozen" => ata.state = AccountState::Frozen,
                    "ata-other-owner" => ata.owner = Pubkey::new_unique(),
                    "ata-other-mint" => ata.mint = Pubkey::new_unique(),
                    _ => {}
                }
                let mut data = vec![0; TokenAccount::LEN];
                TokenAccount::pack(ata, &mut data).unwrap();
                if fault == "ata-malformed" {
                    data.pop();
                }
                *get_mut(&mut case.accounts, case.ata) = account(token::ID, data);
            }
        }
        let result = vm.process_instruction(&case.instruction, &case.accounts);
        assert!(
            result.raw_result.is_ok(),
            "{fault}: {:?}",
            result.raw_result
        );
        let status: TransferStatus = state(&result.resulting_accounts, case.status);
        if fault == "vault-nonsystem" {
            assert_resolution(&result.resulting_accounts, &case, Some(1));
        } else {
            assert!(matches!(
                status.outcome,
                TransferOutcome::Voided {
                    reason: VoidReason::InvalidRecipient
                }
            ));
            assert_eq!(
                get(&result.resulting_accounts, case.mint),
                get(&case.accounts, case.mint)
            );
            assert_eq!(
                get(&result.resulting_accounts, case.ata),
                get(&case.accounts, case.ata)
            );
            assert_eq!(
                get(&result.resulting_accounts, case.bridge),
                get(&case.accounts, case.bridge)
            );
        }
        println!(
            "recipient case={fault} outcome={}",
            if fault == "vault-nonsystem" {
                "minted"
            } else {
                "voided-invalid-recipient"
            }
        );
    }
    let mut rollover = Case::new(fixture);
    bridge_update(&mut rollover, |bridge| bridge.minted_today = u64::MAX);
    let mut client: ClientState = state(&rollover.accounts, rollover.client);
    client.trusting_period = 100_000;
    client.unbonding_period = 200_000;
    set(&mut rollover.accounts, rollover.client, &client);
    vm.sysvars.clock.unix_timestamp = 86_400;
    let result = vm.process_instruction(&rollover.instruction, &rollover.accounts);
    assert!(result.raw_result.is_ok());
    assert_resolution(&result.resulting_accounts, &rollover, Some(1));
    let bridge: BridgeConfig = state(&result.resulting_accounts, rollover.bridge);
    assert_eq!(bridge.mint_day, 1);
    vm.sysvars.clock.unix_timestamp = 105;
    println!("mint daily_rollover=verified");
}

pub(super) fn races(vm: &Mollusk, mint_fixture: &Fixture, settlement: &Fixture) {
    let mut mint = Case::new(mint_fixture);
    let void_value = vec![16, 4, 74, 4, 8, 2, 16, 3];
    let void_fixture = reprove(settlement, settlement_key(&mint.id, 3).unwrap(), void_value);
    let void = Case::new(&void_fixture);
    assert_eq!(mint.status, void.status);
    let old_height = HEIGHT - 1;
    let mut consensus: ConsensusStateStore = state(&mint.accounts, mint.consensus);
    consensus.height = old_height;
    set(&mut mint.accounts, mint.consensus, &consensus);
    let old_key = Pubkey::find_program_address(
        &[ICS07_CONSENSUS_STATE_SEED, &old_height.to_le_bytes()],
        &lc::ID,
    )
    .0;
    mint.replace_key(mint.consensus, old_key);
    mint.consensus = old_key;
    let len = mint.instruction.data.len();
    mint.instruction.data[len - 8..].copy_from_slice(&old_height.to_le_bytes());
    let mut accounts = mint.accounts.clone();
    for entry in &void.accounts {
        if !accounts.iter().any(|existing| existing.0 == entry.0) {
            accounts.push(entry.clone());
        }
    }
    for (name, winner, loser, amount) in [
        ("mint-first", &mint, &void, Some(1)),
        ("void-first", &void, &mint, None),
    ] {
        let result = vm.process_instruction(&winner.instruction, &accounts);
        assert!(result.raw_result.is_ok(), "{name}: {:?}", result.raw_result);
        assert_resolution(&result.resulting_accounts, winner, amount);
        let sealed = result.resulting_accounts;
        for attempt in [winner, loser] {
            let rejected = vm.process_instruction(&attempt.instruction, &sealed);
            assert!(rejected.raw_result.is_err());
            assert_eq!(rejected.resulting_accounts, sealed);
        }
        let close = Instruction {
            program_id: akt_solana::ID,
            accounts: akt_solana::accounts::CloseProofBuffer {
                submitter: winner.payer,
                proof_buffer: winner.buffer,
            }
            .to_account_metas(None),
            data: akt_solana::instruction::CloseProofBuffer {}.data(),
        };
        let closed = vm.process_instruction(&close, &sealed);
        assert!(closed.raw_result.is_ok());
        assert_eq!(
            get(&closed.resulting_accounts, winner.status),
            get(&sealed, winner.status)
        );
        let loser_result = vm.process_instruction(&loser.instruction, &closed.resulting_accounts);
        assert!(loser_result.raw_result.is_err());
        assert_eq!(loser_result.resulting_accounts, closed.resulting_accounts);
        println!("race order={name} winner_committed=true winner_replay=rejected loser=rejected proof_closed=true permanent_status=unchanged");
    }
}

pub(super) fn cpi_rejection(vm: &mut Mollusk, elf: &[u8], mint: &Fixture, settlement: &Fixture) {
    let proxy = pubkey!("CtQLLKbDMt1XVNXtLKJEt1K8cstbckjqE6zyFqR37KTc");
    vm.add_program_with_elf_and_loader(&proxy, elf, &mollusk_svm::program::loader_keys::LOADER_V3);
    for fixture in [mint, settlement] {
        let mut case = Case::new(fixture);
        let inner = case.instruction.clone();
        let mut data = akt_solana::wire::proof_nonce(b"global:proxy_cpi")[..8].to_vec();
        data.extend_from_slice(&(inner.data.len() as u32).to_le_bytes());
        data.extend_from_slice(&inner.data);
        data.extend_from_slice(&(inner.accounts.len() as u32).to_le_bytes());
        for meta in &inner.accounts {
            data.extend_from_slice(&[u8::from(meta.is_signer), u8::from(meta.is_writable)]);
        }
        let mut accounts = vec![
            solana_instruction::AccountMeta::new_readonly(akt_solana::ID, false),
            solana_instruction::AccountMeta::new(case.payer, true),
        ];
        accounts.extend(inner.accounts);
        case.accounts.push((
            akt_solana::ID,
            Account {
                executable: true,
                ..account(mollusk_svm::program::loader_keys::LOADER_V3, vec![])
            },
        ));
        case.instruction = Instruction {
            program_id: proxy,
            accounts,
            data,
        };
        let result = vm.process_instruction(&case.instruction, &case.accounts);
        assert_eq!(
            result.raw_result,
            Err(solana_instruction::error::InstructionError::Custom(7018))
        );
        assert_eq!(result.resulting_accounts, case.accounts);
        println!(
            "cpi name={} rejected=CpiNotAllowed rollback=unchanged",
            fixture.name
        );
    }
}

pub(super) fn heaps(vm: &mut Mollusk, fixtures: &[Fixture]) {
    for heap in [32, 64, 128, 256] {
        vm.compute_budget.heap_size = heap * 1024;
        for fixture in fixtures.iter().filter(|fixture| {
            matches!(
                fixture.name.as_str(),
                "sdk-settlement-maximum_evidence"
                    | "sdk-settlement-64_reporters_32_byte_addresses"
                    | "verifier-depth-127"
                    | "mint-depth-127"
            )
        }) {
            let case = Case::new(fixture);
            let mut runs = vec![];
            let mut successes = 0;
            let mut failure = None;
            for _ in 0..5 {
                let result = vm.process_instruction(&case.instruction, &case.accounts);
                if result.raw_result.is_ok() {
                    successes += 1;
                    assert_resolution(
                        &result.resulting_accounts,
                        &case,
                        fixture
                            .amount
                            .as_ref()
                            .map(|amount| amount.parse().unwrap()),
                    );
                    runs.push(result.compute_units_consumed);
                } else {
                    failure = Some(format!("{:?}", result.raw_result));
                    assert_eq!(result.resulting_accounts, case.accounts);
                }
            }
            if heap == 256 {
                assert_eq!(successes, 5);
            }
            println!(
                "heap name={} heap_kib={heap} successful_runs={successes}/5 successful_cu={runs:?} failure={failure:?}",
                fixture.name
            );
        }
    }
    vm.compute_budget.heap_size = RESOLUTION_HEAP;
}
