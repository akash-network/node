use akt_solana::{
    constants::*,
    state::{
        BridgeConfig, LaunchConfig, MintPolicy, ProofBuffer, TransferOutcome, TransferStatus,
        VoidReason,
    },
    wire::{proof_nonce, record_value, settlement_key, TransferId},
};
use anchor_lang::{
    prelude::*,
    solana_program::{program_option::COption, program_pack::Pack},
    AccountDeserialize, AccountSerialize, InstructionData, ToAccountMetas,
};
use anchor_spl::token::spl_token::state::{Account as TokenAccount, AccountState, Mint};
use ibc_core_commitment_types::merkle::MerkleProof;
use ibc_proto::{
    ibc::core::commitment::v1::MerkleProof as RawMerkleProof, ics23::commitment_proof::Proof,
    Protobuf,
};
use ics07_tendermint_interface::ics07_tendermint as lc;
use lc::{
    accounts::{ClientState, ConsensusStateStore},
    types::{ConsensusState, IbcHeight},
};
use mollusk_svm::Mollusk;
use mollusk_svm_programs_token::{associated_token, token};
use serde::Deserialize;
use solana_account::Account;
use solana_instruction::Instruction;
use std::{env, fs};

mod buffer;
mod resolution;

const HEIGHT: u64 = i64::MAX as u64 + 1;
const PAYER_BALANCE: u64 = 10_000_000_000;
const RESOLUTION_COMPUTE: u64 = 1_400_000;
const RESOLUTION_HEAP: u32 = 256 * 1024;

type Accounts = Vec<(Pubkey, Account)>;

#[derive(Clone, Deserialize)]
struct Fixture {
    name: String,
    root: String,
    key: String,
    value: String,
    proof: String,
    recipient: Option<String>,
    amount: Option<String>,
}

fn bytes(value: &str) -> Vec<u8> {
    hex::decode(value).unwrap()
}

fn serialized<T: AccountSerialize>(value: &T) -> Vec<u8> {
    let mut data = vec![];
    value.try_serialize(&mut data).unwrap();
    data
}

fn account(owner: Pubkey, data: Vec<u8>) -> Account {
    Account {
        lamports: Rent::default().minimum_balance(data.len()),
        data,
        owner,
        executable: false,
        rent_epoch: 0,
    }
}

fn system() -> (Pubkey, Account) {
    (
        anchor_lang::system_program::ID,
        Account {
            executable: true,
            owner: mollusk_svm::program::loader_keys::NATIVE_LOADER,
            ..Account::default()
        },
    )
}

fn get(accounts: &Accounts, key: Pubkey) -> &Account {
    &accounts.iter().find(|entry| entry.0 == key).unwrap().1
}

fn get_mut(accounts: &mut Accounts, key: Pubkey) -> &mut Account {
    &mut accounts.iter_mut().find(|entry| entry.0 == key).unwrap().1
}

fn state<T: AccountDeserialize>(accounts: &Accounts, key: Pubkey) -> T {
    T::try_deserialize(&mut get(accounts, key).data.as_slice()).unwrap()
}

fn set<T: AccountSerialize>(accounts: &mut Accounts, key: Pubkey, value: &T) {
    let target = get_mut(accounts, key);
    let mut data = serialized(value);
    data.resize(data.len().max(target.data.len()), 0);
    target.data = data;
}

#[derive(Clone)]
struct Case {
    instruction: Instruction,
    accounts: Accounts,
    id: TransferId,
    payer: Pubkey,
    buffer: Pubkey,
    status: Pubkey,
    bridge: Pubkey,
    config: Pubkey,
    client: Pubkey,
    latest: Pubkey,
    consensus: Pubkey,
    mint: Pubkey,
    ata: Pubkey,
    recipient: Pubkey,
}

impl Case {
    fn new(fixture: &Fixture) -> Self {
        let program = akt_solana::ID;
        let key = bytes(&fixture.key);
        let start = key.len() - 36;
        let id = TransferId {
            owner: key[start..start + 20].try_into().unwrap(),
            height: u64::from_be_bytes(key[start + 20..start + 28].try_into().unwrap()),
            sequence: u64::from_be_bytes(key[start + 28..].try_into().unwrap()),
        };
        let payer = Pubkey::new_from_array([19; 32]);
        let (config, bump) = Pubkey::find_program_address(&[CONFIG_SEED], &program);
        let bridge = Pubkey::find_program_address(&[BRIDGE_SEED], &program).0;
        let mint = Pubkey::find_program_address(&[MINT_SEED], &program).0;
        let (mint_authority, mint_authority_bump) =
            Pubkey::find_program_address(&[MINT_AUTHORITY_SEED], &program);
        let nonce = proof_nonce(&key);
        let buffer =
            Pubkey::find_program_address(&[PROOF_SEED, payer.as_ref(), &nonce], &program).0;
        let status = Pubkey::find_program_address(
            &[
                TRANSFER_SEED,
                &id.owner,
                &id.height.to_le_bytes(),
                &id.sequence.to_le_bytes(),
            ],
            &program,
        )
        .0;
        let client = Pubkey::find_program_address(&[ICS07_CLIENT_SEED], &lc::ID).0;
        let consensus = Pubkey::find_program_address(
            &[ICS07_CONSENSUS_STATE_SEED, &HEIGHT.to_le_bytes()],
            &lc::ID,
        )
        .0;
        let latest = Pubkey::find_program_address(
            &[ICS07_CONSENSUS_STATE_SEED, &(HEIGHT + 1).to_le_bytes()],
            &lc::ID,
        )
        .0;
        let recipient = fixture
            .recipient
            .as_ref()
            .map(|recipient| Pubkey::new_from_array(bytes(recipient).try_into().unwrap()))
            .unwrap_or(Pubkey::new_from_array([255; 32]));
        let ata = anchor_spl::associated_token::get_associated_token_address(&recipient, &mint);
        let bridge_state = BridgeConfig {
            light_client_program: lc::ID,
            chain_id: "akash-probe-1".into(),
            policy: MintPolicy {
                per_transfer_max: u64::MAX,
                daily_mint_cap: u64::MAX,
                vaults: vec![recipient],
            },
            mint_day: 0,
            minted_today: 0,
        };
        let proof = bytes(&fixture.proof);
        let proof_buffer = ProofBuffer {
            submitter: payer,
            nonce,
            expected_len: proof.len() as u32,
            data: proof,
        };
        let client_state = ClientState {
            chain_id: "akash-probe-1".into(),
            trust_level_numerator: 1,
            trust_level_denominator: 3,
            trusting_period: 100,
            unbonding_period: 200,
            max_clock_drift: 5,
            frozen_height: IbcHeight {
                revision_number: 0,
                revision_height: 0,
            },
            latest_height: IbcHeight {
                revision_number: 1,
                revision_height: HEIGHT + 1,
            },
        };
        let consensus_state = |height| ConsensusStateStore {
            height,
            consensus_state: ConsensusState {
                timestamp: 100_000_000_000,
                root: bytes(&fixture.root).try_into().unwrap(),
                next_validators_hash: [8; 32],
            },
        };
        let mut accounts = vec![
            (
                payer,
                Account {
                    lamports: PAYER_BALANCE,
                    ..Account::default()
                },
            ),
            (bridge, account(program, serialized(&bridge_state))),
            (buffer, account(program, serialized(&proof_buffer))),
            (status, Account::default()),
            (
                lc::ID,
                Account {
                    executable: true,
                    ..account(mollusk_svm::program::loader_keys::LOADER_V3, vec![])
                },
            ),
            (client, account(lc::ID, serialized(&client_state))),
            (
                latest,
                account(lc::ID, serialized(&consensus_state(HEIGHT + 1))),
            ),
            (
                consensus,
                account(lc::ID, serialized(&consensus_state(HEIGHT))),
            ),
            system(),
        ];
        let instruction = if let Some(amount) = &fixture.amount {
            let amount = amount.parse().unwrap();
            assert_eq!(
                record_value(&recipient, amount).unwrap(),
                bytes(&fixture.value)
            );
            let launch = LaunchConfig {
                version: LAUNCH_CONFIG_VERSION,
                initializer: payer,
                mint,
                bridge_enabled: true,
                initialized_at_slot: 1,
                bump,
                mint_authority_bump,
            };
            let mint_state = Mint {
                mint_authority: COption::Some(mint_authority),
                supply: 0,
                decimals: AKT_DECIMALS,
                is_initialized: true,
                freeze_authority: COption::None,
            };
            let mut data = vec![0; Mint::LEN];
            Mint::pack(mint_state, &mut data).unwrap();
            accounts.extend([
                (config, account(program, serialized(&launch))),
                (mint, account(token::ID, data)),
                (mint_authority, Account::default()),
                (recipient, Account::default()),
                (ata, Account::default()),
                token::keyed_account(),
                associated_token::keyed_account(),
            ]);
            Instruction {
                program_id: program,
                accounts: akt_solana::accounts::MintTransfer {
                    payer,
                    config,
                    bridge,
                    proof_buffer: buffer,
                    status,
                    light_client_program: lc::ID,
                    client_state: client,
                    latest_consensus: latest,
                    proof_consensus: consensus,
                    mint,
                    mint_authority,
                    recipient,
                    recipient_token_account: ata,
                    token_program: token::ID,
                    associated_token_program: associated_token::ID,
                    system_program: anchor_lang::system_program::ID,
                }
                .to_account_metas(None),
                data: akt_solana::instruction::MintTransfer {
                    id,
                    recipient_key: recipient,
                    amount,
                    proof_height: HEIGHT,
                }
                .data(),
            }
        } else {
            Instruction {
                program_id: program,
                accounts: akt_solana::accounts::VoidTransfer {
                    payer,
                    bridge,
                    proof_buffer: buffer,
                    status,
                    light_client_program: lc::ID,
                    client_state: client,
                    latest_consensus: latest,
                    proof_consensus: consensus,
                    system_program: anchor_lang::system_program::ID,
                }
                .to_account_metas(None),
                data: akt_solana::instruction::VoidTransfer {
                    id,
                    settlement_status: key[2],
                    proof_height: HEIGHT,
                }
                .data(),
            }
        };
        Self {
            instruction,
            accounts,
            id,
            payer,
            buffer,
            status,
            bridge,
            config,
            client,
            latest,
            consensus,
            mint,
            ata,
            recipient,
        }
    }

    fn replace_key(&mut self, old: Pubkey, new: Pubkey) {
        self.accounts
            .iter_mut()
            .find(|entry| entry.0 == old)
            .unwrap()
            .0 = new;
        for meta in &mut self.instruction.accounts {
            if meta.pubkey == old {
                meta.pubkey = new;
            }
        }
    }

    fn proof(&mut self, mutate: impl FnOnce(&mut MerkleProof)) {
        let mut buffer: ProofBuffer = state(&self.accounts, self.buffer);
        let mut proof =
            <MerkleProof as Protobuf<RawMerkleProof>>::decode_vec(&buffer.data).unwrap();
        mutate(&mut proof);
        buffer.data = proof.encode_vec();
        buffer.expected_len = buffer.data.len() as u32;
        set(&mut self.accounts, self.buffer, &buffer);
    }
}

fn reprove(fixture: &Fixture, key: Vec<u8>, value: Vec<u8>) -> Fixture {
    let mut result = fixture.clone();
    let mut proof =
        <MerkleProof as Protobuf<RawMerkleProof>>::decode_vec(&bytes(&fixture.proof)).unwrap();
    let Some(Proof::Exist(leaf)) = &mut proof.proofs[0].proof else {
        panic!("IAVL proof")
    };
    leaf.key = key.clone();
    leaf.value = value.clone();
    let iavl_root = ics23::calculate_existence_root::<ics23::HostFunctionsManager>(leaf).unwrap();
    let Some(Proof::Exist(store)) = &mut proof.proofs[1].proof else {
        panic!("multistore proof")
    };
    store.value = iavl_root;
    result.root =
        hex::encode(ics23::calculate_existence_root::<ics23::HostFunctionsManager>(store).unwrap());
    result.proof = hex::encode(proof.encode_vec());
    result.key = hex::encode(key);
    result.value = hex::encode(value);
    result
}

fn assert_resolution(accounts: &Accounts, case: &Case, amount: Option<u64>) {
    let status: TransferStatus = state(accounts, case.status);
    assert_eq!(status.id, case.id);
    assert_eq!(
        status.proof_height,
        u64::from_le_bytes(
            case.instruction.data[case.instruction.data.len() - 8..]
                .try_into()
                .unwrap()
        )
    );
    assert_eq!(status.resolved_at_slot, 500);
    match (amount, status.outcome) {
        (
            Some(amount),
            TransferOutcome::Minted {
                amount: observed,
                recipient,
            },
        ) => {
            assert_eq!(observed, amount);
            assert_eq!(recipient, case.recipient);
            assert_eq!(
                Mint::unpack(&get(accounts, case.mint).data).unwrap().supply,
                amount
            );
            let ata = TokenAccount::unpack(&get(accounts, case.ata).data).unwrap();
            assert_eq!(ata.amount, amount);
            assert_eq!(ata.owner, recipient);
            let bridge: BridgeConfig = state(accounts, case.bridge);
            assert_eq!(bridge.minted_today, amount);
        }
        (None, TransferOutcome::Voided { .. }) => {}
        _ => panic!("unexpected terminal outcome"),
    }
}

fn successful_resolutions(vm: &mut Mollusk, fixtures: &[Fixture]) {
    for fixture in fixtures {
        let case = Case::new(fixture);
        let mut runs = vec![];
        for _ in 0..5 {
            let result = vm.process_instruction(&case.instruction, &case.accounts);
            assert!(
                result.raw_result.is_ok(),
                "{} {:?}",
                fixture.name,
                result.raw_result
            );
            assert_resolution(
                &result.resulting_accounts,
                &case,
                fixture
                    .amount
                    .as_ref()
                    .map(|amount| amount.parse().unwrap()),
            );
            assert_eq!(
                get(&result.resulting_accounts, case.buffer),
                get(&case.accounts, case.buffer)
            );
            runs.push(result.compute_units_consumed);
        }
        println!("resolution name={} proof_bytes={} heap_bytes={} compute_budget={} five_cu={runs:?} verified=5", fixture.name, bytes(&fixture.proof).len(), vm.compute_budget.heap_size, RESOLUTION_COMPUTE);
    }
}

fn reject(vm: &Mollusk, name: &str, case: &Case) {
    let result = vm.process_instruction(&case.instruction, &case.accounts);
    assert!(result.raw_result.is_err(), "accepted {name}");
    assert_eq!(result.resulting_accounts, case.accounts, "rollback {name}");
    println!(
        "reject name={name} cu={} result={:?} rollback=unchanged",
        result.compute_units_consumed, result.raw_result
    );
}

fn negative_membership(vm: &mut Mollusk, fixture: &Fixture) {
    for fault in [
        "nonce-last-byte",
        "nonce-bound-pda",
        "incomplete",
        "buffer-owner",
        "buffer-pda",
        "buffer-discriminator",
        "buffer-submitter",
        "root",
        "client-owner",
        "client-pda",
        "client-discriminator",
        "latest-owner",
        "latest-pda",
        "latest-discriminator",
        "latest-height",
        "consensus-owner",
        "consensus-pda",
        "consensus-discriminator",
        "consensus-height",
        "program-executable",
        "program-pin",
        "frozen-number",
        "frozen-height",
        "wrong-chain",
        "zero-trust",
        "trust-equals-unbond",
        "expired",
        "future",
        "negative-clock",
        "zero-timestamp",
        "future-proof",
        "zero-proof",
        "same-height",
        "leaf",
        "nonreceipt-value-tamper",
        "omit-iavl",
        "omit-store",
        "reverse",
        "wrong-key",
        "key-bound-nonce",
        "invalid-status",
    ] {
        let mut case = Case::new(fixture);
        vm.sysvars.clock.unix_timestamp = 105;
        match fault {
            "nonce-last-byte" => get_mut(&mut case.accounts, case.buffer).data[71] ^= 1,
            "nonce-bound-pda" => {
                let mut buffer: ProofBuffer = state(&case.accounts, case.buffer);
                buffer.nonce[31] ^= 1;
                set(&mut case.accounts, case.buffer, &buffer);
                let address = Pubkey::find_program_address(
                    &[PROOF_SEED, case.payer.as_ref(), &buffer.nonce],
                    &akt_solana::ID,
                )
                .0;
                case.replace_key(case.buffer, address);
            }
            "incomplete" => get_mut(&mut case.accounts, case.buffer).data[72] ^= 1,
            "buffer-owner" => get_mut(&mut case.accounts, case.buffer).owner = Pubkey::new_unique(),
            "buffer-pda" => case.replace_key(case.buffer, Pubkey::new_unique()),
            "buffer-discriminator" => get_mut(&mut case.accounts, case.buffer).data[0] ^= 1,
            "buffer-submitter" => get_mut(&mut case.accounts, case.buffer).data[8] ^= 1,
            "root" => get_mut(&mut case.accounts, case.consensus).data[24] ^= 1,
            "client-owner" => get_mut(&mut case.accounts, case.client).owner = Pubkey::new_unique(),
            "client-pda" => case.replace_key(case.client, Pubkey::new_unique()),
            "client-discriminator" => get_mut(&mut case.accounts, case.client).data[0] ^= 1,
            "latest-owner" => get_mut(&mut case.accounts, case.latest).owner = Pubkey::new_unique(),
            "latest-pda" => case.replace_key(case.latest, Pubkey::new_unique()),
            "latest-discriminator" => get_mut(&mut case.accounts, case.latest).data[0] ^= 1,
            "latest-height" => get_mut(&mut case.accounts, case.latest).data[8] ^= 1,
            "consensus-owner" => {
                get_mut(&mut case.accounts, case.consensus).owner = Pubkey::new_unique()
            }
            "consensus-pda" => case.replace_key(case.consensus, Pubkey::new_unique()),
            "consensus-discriminator" => get_mut(&mut case.accounts, case.consensus).data[0] ^= 1,
            "consensus-height" => get_mut(&mut case.accounts, case.consensus).data[8] ^= 1,
            "program-executable" => get_mut(&mut case.accounts, lc::ID).executable = false,
            "program-pin" => get_mut(&mut case.accounts, case.bridge).data[8] ^= 1,
            "frozen-number"
            | "frozen-height"
            | "wrong-chain"
            | "zero-trust"
            | "trust-equals-unbond" => {
                let mut client: ClientState = state(&case.accounts, case.client);
                match fault {
                    "frozen-number" => client.frozen_height.revision_number = 1,
                    "frozen-height" => client.frozen_height.revision_height = 1,
                    "wrong-chain" => client.chain_id = "other-chain".into(),
                    "zero-trust" => client.trusting_period = 0,
                    _ => client.trusting_period = client.unbonding_period,
                }
                set(&mut case.accounts, case.client, &client);
            }
            "expired" => vm.sysvars.clock.unix_timestamp = 200,
            "future" => vm.sysvars.clock.unix_timestamp = 94,
            "negative-clock" => vm.sysvars.clock.unix_timestamp = -1,
            "zero-timestamp" => get_mut(&mut case.accounts, case.latest).data[16..24].fill(0),
            "future-proof" | "zero-proof" | "same-height" => {
                let height = match fault {
                    "future-proof" => HEIGHT + 2,
                    "same-height" => case.id.height,
                    _ => 0,
                };
                let len = case.instruction.data.len();
                case.instruction.data[len - 8..].copy_from_slice(&height.to_le_bytes());
            }
            "leaf" | "nonreceipt-value-tamper" | "omit-iavl" | "omit-store" | "reverse" => case
                .proof(|proof| match fault {
                    "leaf" => {
                        if let Some(Proof::Exist(leaf)) = &mut proof.proofs[0].proof {
                            leaf.leaf.as_mut().unwrap().hash = 0;
                        }
                    }
                    "nonreceipt-value-tamper" => {
                        if let Some(Proof::Exist(leaf)) = &mut proof.proofs[0].proof {
                            leaf.value[6000] ^= 1;
                        }
                    }
                    "omit-iavl" => {
                        proof.proofs.remove(0);
                    }
                    "omit-store" => {
                        proof.proofs.pop();
                    }
                    _ => proof.proofs.reverse(),
                }),
            "wrong-key" => case.instruction.data[44] = 5,
            "key-bound-nonce" => {
                case.instruction.data[44] = 5;
                let mut buffer: ProofBuffer = state(&case.accounts, case.buffer);
                buffer.nonce = proof_nonce(&settlement_key(&case.id, 5).unwrap());
                set(&mut case.accounts, case.buffer, &buffer);
                let next = Pubkey::find_program_address(
                    &[PROOF_SEED, case.payer.as_ref(), &buffer.nonce],
                    &akt_solana::ID,
                )
                .0;
                case.replace_key(case.buffer, next);
            }
            "invalid-status" => case.instruction.data[44] = 6,
            _ => unreachable!(),
        }
        reject(vm, fault, &case);
    }
    vm.sysvars.clock.unix_timestamp = 105;
}

fn receipt_faults(vm: &Mollusk, fixture: &Fixture) {
    for (name, value) in [
        ("missing-receipt", vec![16, 4]),
        (
            "duplicate-receipt",
            vec![16, 4, 74, 4, 8, 2, 16, 3, 74, 4, 8, 2, 16, 3],
        ),
        ("duplicate-round", vec![16, 4, 16, 4, 74, 4, 8, 2, 16, 3]),
        ("nonminimal-height", vec![16, 4, 74, 5, 8, 0x82, 0, 16, 3]),
        (
            "overflow-round",
            vec![
                16, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 2, 74, 4, 8, 2, 16, 3,
            ],
        ),
        (
            "negative-height",
            vec![
                16, 4, 74, 13, 8, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 1, 16, 3,
            ],
        ),
        ("future-round", vec![16, 4, 74, 4, 8, 2, 16, 5]),
        ("wrong-wire", vec![16, 4, 74, 4, 9, 2, 16, 3]),
        ("truncated-receipt", vec![16, 4, 74, 5, 8, 2, 16, 3]),
        ("unknown-tag", vec![16, 4, 74, 4, 8, 2, 16, 3, 90, 0]),
    ] {
        let proven = reprove(fixture, bytes(&fixture.key), value);
        reject(vm, name, &Case::new(&proven));
    }
    for status in 1..=5 {
        let mut key = bytes(&fixture.key);
        key[2] = status;
        let proven = reprove(fixture, key, vec![16, 4, 74, 4, 8, 2, 16, 3]);
        let case = Case::new(&proven);
        let result = vm.process_instruction(&case.instruction, &case.accounts);
        assert!(
            result.raw_result.is_ok(),
            "status {status}: {:?}",
            result.raw_result
        );
        assert_resolution(&result.resulting_accounts, &case, None);
        println!("permanent-receipt status={status} older_round=accepted full_membership=verified");
    }
}

fn main() {
    let args: Vec<String> = env::args().collect();
    let mut fixtures: Vec<Fixture> = serde_json::from_slice(&fs::read(&args[1]).unwrap()).unwrap();
    let elf = fs::read(&args[2]).unwrap();
    let mut vm = Mollusk::default();
    vm.compute_budget.compute_unit_limit = RESOLUTION_COMPUTE;
    vm.compute_budget.heap_size = RESOLUTION_HEAP;
    vm.sysvars.clock.unix_timestamp = 105;
    vm.sysvars.clock.slot = 500;
    vm.add_program_with_elf_and_loader(
        &akt_solana::ID,
        &elf,
        &mollusk_svm::program::loader_keys::LOADER_V3,
    );
    token::add_program(&mut vm);
    associated_token::add_program(&mut vm);
    let records: Vec<serde_json::Value> =
        serde_json::from_str(include_str!("../../fixtures/proof_vectors.json")).unwrap();
    let settlements: Vec<serde_json::Value> =
        serde_json::from_str(include_str!("../../fixtures/settlement_vectors.json")).unwrap();
    let mint_template = fixtures
        .iter()
        .find(|fixture| fixture.name == "mint-1")
        .unwrap()
        .clone();
    let void_template = fixtures
        .iter()
        .find(|fixture| fixture.name == "certificate-clock-bound")
        .unwrap()
        .clone();
    for record in records {
        let mut fixture = reprove(
            &mint_template,
            bytes(record["transfer_key_hex"].as_str().unwrap()),
            bytes(record["record_value_hex"].as_str().unwrap()),
        );
        fixture.name = format!("sdk-mint-{}", record["name"].as_str().unwrap());
        fixture.recipient = Some(record["recipient_hex"].as_str().unwrap().to_string());
        fixture.amount = Some(record["amount"].as_str().unwrap().to_string());
        fixtures.push(fixture);
    }
    for settlement in settlements {
        let mut fixture = reprove(
            &void_template,
            bytes(settlement["settlement_key_hex"].as_str().unwrap()),
            bytes(settlement["settlement_value_hex"].as_str().unwrap()),
        );
        fixture.name = format!("sdk-settlement-{}", settlement["name"].as_str().unwrap());
        if settlement["settlement"]["void_authorization"].is_null() {
            reject(&vm, &fixture.name, &Case::new(&fixture));
        } else {
            fixtures.push(fixture);
        }
    }
    let depth = fixtures
        .iter()
        .find(|fixture| fixture.name == "verifier-depth-127")
        .unwrap();
    let maximum = fixtures
        .iter()
        .find(|fixture| fixture.name == "sdk-mint-maximum")
        .unwrap();
    let mut mint_depth = reprove(depth, bytes(&maximum.key), bytes(&maximum.value));
    mint_depth.name = "mint-depth-127".into();
    mint_depth.recipient = maximum.recipient.clone();
    mint_depth.amount = maximum.amount.clone();
    fixtures.push(mint_depth);
    successful_resolutions(&mut vm, &fixtures);
    let settlement = fixtures
        .iter()
        .find(|fixture| fixture.name == "certificate-clock-bound")
        .unwrap();
    negative_membership(&mut vm, settlement);
    receipt_faults(&vm, settlement);
    buffer::exercise(&mut vm);
    resolution::mint_checks(&mut vm, &mint_template);
    resolution::races(&vm, &mint_template, settlement);
    resolution::heaps(&mut vm, &fixtures);
    let proxy = fs::read(&args[3]).unwrap();
    resolution::cpi_rejection(&mut vm, &proxy, &mint_template, settlement);
    println!("unexpected_failures=0");
}
