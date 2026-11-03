use super::*;

fn instruction(accounts: impl ToAccountMetas, data: impl InstructionData) -> Instruction {
    Instruction {
        program_id: akt_solana::ID,
        accounts: accounts.to_account_metas(None),
        data: data.data(),
    }
}

pub(super) fn exercise(vm: &mut Mollusk) {
    let payer = Pubkey::new_from_array([51; 32]);
    for length in [0, 1, 4096, 10_160, 10_161, 16_384, 16_385] {
        let nonce = [length as u8; 32];
        let buffer =
            Pubkey::find_program_address(&[PROOF_SEED, payer.as_ref(), &nonce], &akt_solana::ID).0;
        let accounts = vec![
            (
                payer,
                Account {
                    lamports: PAYER_BALANCE,
                    ..Account::default()
                },
            ),
            (buffer, Account::default()),
            system(),
        ];
        let create = instruction(
            akt_solana::accounts::CreateProofBuffer {
                submitter: payer,
                proof_buffer: buffer,
                system_program: anchor_lang::system_program::ID,
            },
            akt_solana::instruction::CreateProofBuffer {
                nonce,
                expected_len: length,
            },
        );
        let result = vm.process_instruction(&create, &accounts);
        if length == 0 || length > 16_384 {
            assert!(result.raw_result.is_err());
            assert_eq!(result.resulting_accounts, accounts);
            println!("buffer invalid_length={length} rollback=unchanged");
            continue;
        }
        assert!(
            result.raw_result.is_ok(),
            "create {length} {:?}",
            result.raw_result
        );
        let created = (80 + length as usize).min(10_240);
        assert_eq!(get(&result.resulting_accounts, buffer).data.len(), created);
        assert_eq!(
            get(&result.resulting_accounts, buffer).lamports,
            vm.sysvars.rent.minimum_balance(created)
        );
        let mut accounts = result.resulting_accounts;
        let grow = instruction(
            akt_solana::accounts::GrowProofBuffer {
                submitter: payer,
                proof_buffer: buffer,
                system_program: anchor_lang::system_program::ID,
            },
            akt_solana::instruction::GrowProofBuffer {},
        );
        let append = |offset, chunk| {
            instruction(
                akt_solana::accounts::WriteProofBuffer {
                    submitter: payer,
                    proof_buffer: buffer,
                },
                akt_solana::instruction::AppendProof { offset, chunk },
            )
        };
        if created < 80 + length as usize {
            let mut nearly_full: ProofBuffer = state(&accounts, buffer);
            nearly_full.data.resize(created - 80, 7);
            set(&mut accounts, buffer, &nearly_full);
            let over =
                vm.process_instruction(&append(nearly_full.data.len() as u32, vec![8]), &accounts);
            assert!(over.raw_result.is_err());
            assert_eq!(over.resulting_accounts, accounts);
            nearly_full.data.clear();
            set(&mut accounts, buffer, &nearly_full);
            let mut underfunded = accounts.clone();
            get_mut(&mut underfunded, payer).lamports = 1;
            let failed = vm.process_instruction(&grow, &underfunded);
            assert!(failed.raw_result.is_err());
            assert_eq!(failed.resulting_accounts, underfunded);
        }
        let result = vm.process_instruction(&grow, &accounts);
        assert!(
            result.raw_result.is_ok(),
            "grow {length} {:?}",
            result.raw_result
        );
        accounts = result.resulting_accounts;
        assert_eq!(get(&accounts, buffer).data.len(), 80 + length as usize);
        assert_eq!(
            get(&accounts, buffer).lamports,
            vm.sysvars.rent.minimum_balance(80 + length as usize)
        );
        assert_eq!(
            get(&accounts, payer).lamports + get(&accounts, buffer).lamports,
            PAYER_BALANCE
        );
        let result = vm.process_instruction(&grow, &accounts);
        assert!(result.raw_result.is_ok());
        assert_eq!(result.resulting_accounts, accounts);
        for fault in [
            "wrong-signer",
            "missing-signature",
            "foreign-owner",
            "wrong-pda",
            "nonce-tail",
        ] {
            let mut bad_accounts = accounts.clone();
            let mut bad_grow = grow.clone();
            match fault {
                "wrong-signer" => {
                    let other = Pubkey::new_unique();
                    bad_accounts.push((
                        other,
                        Account {
                            lamports: PAYER_BALANCE,
                            ..Account::default()
                        },
                    ));
                    bad_grow.accounts[0].pubkey = other;
                }
                "missing-signature" => bad_grow.accounts[0].is_signer = false,
                "foreign-owner" => get_mut(&mut bad_accounts, buffer).owner = Pubkey::new_unique(),
                "wrong-pda" => {
                    let other = Pubkey::new_unique();
                    bad_accounts
                        .iter_mut()
                        .find(|entry| entry.0 == buffer)
                        .unwrap()
                        .0 = other;
                    bad_grow.accounts[1].pubkey = other;
                }
                "nonce-tail" => get_mut(&mut bad_accounts, buffer).data[71] ^= 1,
                _ => unreachable!(),
            }
            let close = instruction(
                akt_solana::accounts::CloseProofBuffer {
                    submitter: payer,
                    proof_buffer: buffer,
                },
                akt_solana::instruction::CloseProofBuffer {},
            );
            for mut operation in [grow.clone(), append(0, vec![1]), close] {
                operation.accounts[0] = bad_grow.accounts[0].clone();
                operation.accounts[1] = bad_grow.accounts[1].clone();
                let result = vm.process_instruction(&operation, &bad_accounts);
                assert!(result.raw_result.is_err(), "buffer authority {fault}");
                assert_eq!(result.resulting_accounts, bad_accounts);
            }
        }
        for (offset, chunk) in [(1, vec![1]), (0, vec![]), (0, vec![1; 501])] {
            let result = vm.process_instruction(&append(offset, chunk), &accounts);
            assert!(result.raw_result.is_err());
            assert_eq!(result.resulting_accounts, accounts);
        }
        let proof: Vec<u8> = (0..length).map(|index| (index % 251) as u8).collect();
        let mut append_compute = vec![];
        for chunk in proof.chunks(500) {
            let restored: ProofBuffer = state(&accounts, buffer);
            let offset = restored.data.len() as u32;
            let result = vm.process_instruction(&append(offset, chunk.to_vec()), &accounts);
            assert!(
                result.raw_result.is_ok(),
                "append {length} at {offset}: {:?}",
                result.raw_result
            );
            append_compute.push(result.compute_units_consumed);
            accounts = result.resulting_accounts;
            if offset == 0 {
                let replay = vm.process_instruction(&append(offset, chunk.to_vec()), &accounts);
                assert!(replay.raw_result.is_err());
                assert_eq!(replay.resulting_accounts, accounts);
            }
        }
        let completed: ProofBuffer = state(&accounts, buffer);
        assert!(completed.is_complete());
        assert_eq!(completed.data, proof);
        let result = vm.process_instruction(&append(length, vec![1]), &accounts);
        assert!(result.raw_result.is_err());
        assert_eq!(result.resulting_accounts, accounts);
        let close = instruction(
            akt_solana::accounts::CloseProofBuffer {
                submitter: payer,
                proof_buffer: buffer,
            },
            akt_solana::instruction::CloseProofBuffer {},
        );
        let result = vm.process_instruction(&close, &accounts);
        assert!(result.raw_result.is_ok());
        assert_eq!(
            get(&result.resulting_accounts, payer).lamports,
            PAYER_BALANCE
        );
        assert_eq!(get(&result.resulting_accounts, buffer).lamports, 0);
        assert!(get(&result.resulting_accounts, buffer).data.is_empty());
        let result = vm.process_instruction(&create, &result.resulting_accounts);
        assert!(result.raw_result.is_ok(), "recreate after close");
        let unfinished = vm.process_instruction(&close, &result.resulting_accounts);
        assert!(unfinished.raw_result.is_ok());
        assert_eq!(
            get(&unfinished.resulting_accounts, payer).lamports,
            PAYER_BALANCE
        );
        println!("buffer length={length} initial_bytes={created} grown_bytes={} rent={} chunks={} max_append_cu={} create_grow_resume_close_recreate=passed", 80 + length, vm.sysvars.rent.minimum_balance(80 + length as usize), append_compute.len(), append_compute.iter().max().unwrap());
    }
}
