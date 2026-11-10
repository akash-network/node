/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/akt.json`.
 */
export type Akt = {
  "address": "98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME",
  "metadata": {
    "name": "akt",
    "version": "0.1.0",
    "spec": "0.1.0",
    "description": "AKT on Solana: zero-supply genesis, then mints only for Akash x/sink records verified by an ICS07 light client"
  },
  "docs": [
    "Entrypoints in lifecycle order. Handlers live in `instructions`."
  ],
  "instructions": [
    {
      "name": "appendProof",
      "docs": [
        "Appends `chunk` at `offset` to the caller's proof buffer.",
        "",
        "Caller: the buffer's submitter.",
        "Errors: `InvalidProofOffset`, `InvalidProofLength`."
      ],
      "discriminator": [
        174,
        126,
        237,
        6,
        177,
        246,
        130,
        168
      ],
      "accounts": [
        {
          "name": "submitter",
          "signer": true,
          "relations": [
            "proofBuffer"
          ]
        },
        {
          "name": "proofBuffer",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  112,
                  114,
                  111,
                  111,
                  102
                ]
              },
              {
                "kind": "account",
                "path": "submitter"
              },
              {
                "kind": "account",
                "path": "proof_buffer.nonce",
                "account": "proofBuffer"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "offset",
          "type": "u32"
        },
        {
          "name": "chunk",
          "type": "bytes"
        }
      ]
    },
    {
      "name": "closeProofBuffer",
      "docs": [
        "Closes the caller's proof buffer and returns its rent.",
        "",
        "Caller: the buffer's submitter."
      ],
      "discriminator": [
        130,
        150,
        6,
        35,
        193,
        34,
        243,
        87
      ],
      "accounts": [
        {
          "name": "submitter",
          "docs": [
            "Receives the rent."
          ],
          "writable": true,
          "signer": true,
          "relations": [
            "proofBuffer"
          ]
        },
        {
          "name": "proofBuffer",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  112,
                  114,
                  111,
                  111,
                  102
                ]
              },
              {
                "kind": "account",
                "path": "submitter"
              },
              {
                "kind": "account",
                "path": "proof_buffer.nonce",
                "account": "proofBuffer"
              }
            ]
          }
        }
      ],
      "args": []
    },
    {
      "name": "configureBridge",
      "docs": [
        "Pins the light-client deployment and Akash chain ID and sets the first",
        "mint policy. One-time; minting stays disabled.",
        "",
        "Caller: the upgrade authority. It pays rent.",
        "Event: `BridgeConfigured`.",
        "Errors: `InvalidChainId`, `InvalidPolicy`, `InvalidLightClient`,",
        "`WrongChain`, `ClientFrozen`, `ClientExpired`,",
        "`InvalidConsensusTimestamp`."
      ],
      "discriminator": [
        62,
        212,
        237,
        254,
        204,
        203,
        160,
        208
      ],
      "accounts": [
        {
          "name": "admin",
          "accounts": [
            {
              "name": "program",
              "address": "98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME"
            },
            {
              "name": "programData"
            },
            {
              "name": "authority",
              "writable": true,
              "signer": true
            }
          ]
        },
        {
          "name": "config",
          "docs": [
            "Requiring the launch record orders configuration after genesis."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "bridge",
          "docs": [
            "Created here; `init` makes configuration one-time."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  98,
                  114,
                  105,
                  100,
                  103,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "lightClientProgram",
          "docs": [
            "Must be executable; its client accounts are checked below."
          ]
        },
        {
          "name": "clientState",
          "docs": [
            "type; `check_active` its chain ID, frozen height and trusting period."
          ]
        },
        {
          "name": "latestConsensus",
          "docs": [
            "(the client's latest) and that it is inside the trusting period."
          ]
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "chainId",
          "type": "string"
        },
        {
          "name": "policy",
          "type": {
            "defined": {
              "name": "mintPolicy"
            }
          }
        }
      ]
    },
    {
      "name": "createProofBuffer",
      "docs": [
        "Creates the caller's proof buffer `[\"proof\", submitter, nonce]` for a",
        "proof of `expected_len` bytes.",
        "",
        "Caller: anyone. It pays rent and owns the buffer.",
        "Errors: `InvalidProofLength`."
      ],
      "discriminator": [
        42,
        36,
        226,
        19,
        141,
        70,
        66,
        141
      ],
      "accounts": [
        {
          "name": "submitter",
          "docs": [
            "Owns the buffer and pays its rent."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "proofBuffer",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  112,
                  114,
                  111,
                  111,
                  102
                ]
              },
              {
                "kind": "account",
                "path": "submitter"
              },
              {
                "kind": "arg",
                "path": "nonce"
              }
            ]
          }
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "nonce",
          "type": {
            "array": [
              "u8",
              32
            ]
          }
        },
        {
          "name": "expectedLen",
          "type": "u32"
        }
      ]
    },
    {
      "name": "growProofBuffer",
      "docs": [
        "Grows the caller's proof buffer toward its declared length and funds rent."
      ],
      "discriminator": [
        208,
        119,
        54,
        198,
        121,
        76,
        59,
        94
      ],
      "accounts": [
        {
          "name": "submitter",
          "writable": true,
          "signer": true,
          "relations": [
            "proofBuffer"
          ]
        },
        {
          "name": "proofBuffer",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  112,
                  114,
                  111,
                  111,
                  102
                ]
              },
              {
                "kind": "account",
                "path": "submitter"
              },
              {
                "kind": "account",
                "path": "proof_buffer.nonce",
                "account": "proofBuffer"
              }
            ]
          }
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": []
    },
    {
      "name": "guardianPause",
      "docs": [
        "Disables `mint_transfer`. The guardian cannot enable minting or change",
        "any other setting.",
        "",
        "Caller: the pause guardian.",
        "Event: `GuardianPaused`.",
        "Errors: `UnauthorizedGuardian`."
      ],
      "discriminator": [
        184,
        93,
        27,
        13,
        127,
        100,
        198,
        238
      ],
      "accounts": [
        {
          "name": "guardian",
          "signer": true
        },
        {
          "name": "pauseGuardian",
          "docs": [
            "Must name `guardian`. Before the first `set_pause_guardian` the account",
            "does not exist and every call fails."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  103,
                  117,
                  97,
                  114,
                  100,
                  105,
                  97,
                  110
                ]
              }
            ]
          }
        },
        {
          "name": "config",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        }
      ],
      "args": []
    },
    {
      "name": "initialize",
      "docs": [
        "Genesis: creates the launch record and the AKT mint. Nothing is",
        "minted; supply starts at zero and bridge minting starts disabled.",
        "",
        "Caller: the program's upgrade authority, once. It pays rent.",
        "Event: `LaunchInitialized`.",
        "Errors: `InvalidProgramData`, `UnauthorizedInitializer`. A second call",
        "fails because `config` exists."
      ],
      "discriminator": [
        175,
        175,
        109,
        31,
        13,
        152,
        155,
        237
      ],
      "accounts": [
        {
          "name": "program",
          "docs": [
            "This program. Anchor checks its address and executable flag."
          ],
          "address": "98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME"
        },
        {
          "name": "programData",
          "docs": [
            "This program's ProgramData. Anchor checks the loader owns it; its",
            "recorded upgrade authority must sign."
          ]
        },
        {
          "name": "authority",
          "docs": [
            "Pays rent for every account created here."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "config",
          "docs": [
            "Created here; `init` fails if genesis already ran."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "mint",
          "docs": [
            "Legacy SPL mint with six decimals, the mint-authority PDA as mint",
            "authority and no freeze authority. Its supply starts at zero."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  105,
                  110,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "mintAuthority",
          "docs": [
            "CPIs from this program."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  105,
                  110,
                  116,
                  95,
                  97,
                  117,
                  116,
                  104,
                  111,
                  114,
                  105,
                  116,
                  121
                ]
              }
            ]
          }
        },
        {
          "name": "tokenProgram",
          "address": "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": []
    },
    {
      "name": "mintTransfer",
      "docs": [
        "Resolves an x/sink record: verifies through the pinned client that",
        "the transfer key of `id` holds native RecordData at `proof_height`, then",
        "mints `amount` to the recipient's associated token account, or",
        "voids the record if the recipient cannot hold AKT.",
        "",
        "Caller: anyone, in a top-level instruction preceded only by",
        "ComputeBudget instructions, while minting is enabled. It pays rent for",
        "the status and the token account.",
        "Event: `TransferResolved`.",
        "Errors: `BridgeDisabled`, `CpiNotAllowed`, `PrecedingInstruction`,",
        "`InvalidRecord`, the light-client and proof errors, `InvalidClock`,",
        "`TransferLimit`, `DailyLimit`, and membership verification errors. A",
        "resolved record fails because its status exists."
      ],
      "discriminator": [
        229,
        169,
        199,
        50,
        95,
        117,
        44,
        245
      ],
      "accounts": [
        {
          "name": "payer",
          "docs": [
            "Pays rent for the status account and, on a mint, the recipient's",
            "associated token account if it does not exist."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "config",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "bridge",
          "docs": [
            "Pinned client and policy. A mint updates the day's usage."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  98,
                  114,
                  105,
                  100,
                  103,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "proofBuffer",
          "docs": [
            "A complete proof of the transfer key of `id`, from any",
            "submitter's buffer."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  112,
                  114,
                  111,
                  111,
                  102
                ]
              },
              {
                "kind": "account",
                "path": "proof_buffer.submitter",
                "account": "proofBuffer"
              },
              {
                "kind": "account",
                "path": "proof_buffer.nonce",
                "account": "proofBuffer"
              }
            ]
          }
        },
        {
          "name": "status",
          "docs": [
            "Terminal status. `init` fails if the record already has one."
          ],
          "writable": true
        },
        {
          "name": "lightClientProgram"
        },
        {
          "name": "clientState",
          "docs": [
            "chain ID, frozen height and trusting period."
          ]
        },
        {
          "name": "latestConsensus",
          "docs": [
            "(the client's latest) and that it is inside the trusting period."
          ]
        },
        {
          "name": "proofConsensus",
          "docs": [
            "(`proof_height`); the pinned library verifies against its root."
          ]
        },
        {
          "name": "mint",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  105,
                  110,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "mintAuthority",
          "docs": [
            "data and signs the mint CPI."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  105,
                  110,
                  116,
                  95,
                  97,
                  117,
                  116,
                  104,
                  111,
                  114,
                  105,
                  116,
                  121
                ]
              }
            ]
          }
        },
        {
          "name": "recipient",
          "docs": [
            "checks its owner, executable flag and curve point."
          ]
        },
        {
          "name": "recipientTokenAccount",
          "docs": [
            "the AKT mint. `ata_usable` checks any existing contents before a mint."
          ],
          "writable": true
        },
        {
          "name": "tokenProgram",
          "address": "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
        },
        {
          "name": "associatedTokenProgram",
          "address": "ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "id",
          "type": {
            "defined": {
              "name": "transferId"
            }
          }
        },
        {
          "name": "recipientKey",
          "type": "pubkey"
        },
        {
          "name": "amount",
          "type": "u64"
        },
        {
          "name": "proofHeight",
          "type": "u64"
        }
      ]
    },
    {
      "name": "repinLightClient",
      "docs": [
        "Pins another light-client deployment and chain ID, with the checks of",
        "`configure_bridge`. The current pin is not read, so a broken, expired",
        "or closed deployment cannot block recovery. Policy and usage carry",
        "over.",
        "",
        "Caller: the upgrade authority, at any time.",
        "Event: `LightClientRepinned`.",
        "Errors: those of `configure_bridge` except `InvalidPolicy`."
      ],
      "discriminator": [
        33,
        154,
        83,
        141,
        208,
        155,
        113,
        122
      ],
      "accounts": [
        {
          "name": "admin",
          "accounts": [
            {
              "name": "program",
              "address": "98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME"
            },
            {
              "name": "programData"
            },
            {
              "name": "authority",
              "writable": true,
              "signer": true
            }
          ]
        },
        {
          "name": "bridge",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  98,
                  114,
                  105,
                  100,
                  103,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "lightClientProgram",
          "docs": [
            "authority. Must be executable; its client accounts are checked below."
          ]
        },
        {
          "name": "clientState",
          "docs": [
            "type; `check_active` its chain ID, frozen height and trusting period."
          ]
        },
        {
          "name": "latestConsensus",
          "docs": [
            "(the client's latest) and that it is inside the trusting period."
          ]
        }
      ],
      "args": [
        {
          "name": "chainId",
          "type": "string"
        }
      ]
    },
    {
      "name": "setBridgeEnabled",
      "docs": [
        "Enables or disables `mint_transfer`. `void_transfer` is never gated.",
        "",
        "Caller: the upgrade authority, after `configure_bridge`.",
        "Event: `BridgeEnabledSet`.",
        "Errors: `InvalidAuthority`."
      ],
      "discriminator": [
        177,
        240,
        21,
        76,
        138,
        104,
        176,
        29
      ],
      "accounts": [
        {
          "name": "admin",
          "accounts": [
            {
              "name": "program",
              "address": "98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME"
            },
            {
              "name": "programData"
            },
            {
              "name": "authority",
              "writable": true,
              "signer": true
            }
          ]
        },
        {
          "name": "config",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "bridge",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  98,
                  114,
                  105,
                  100,
                  103,
                  101
                ]
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "enabled",
          "type": "bool"
        }
      ]
    },
    {
      "name": "setMintPolicy",
      "docs": [
        "Replaces the per-transfer limit, daily cap and vault allowlist. The",
        "day's minted total carries over.",
        "",
        "Caller: the upgrade authority.",
        "Event: `MintPolicySet`.",
        "Errors: `InvalidAuthority`, `InvalidPolicy`."
      ],
      "discriminator": [
        12,
        208,
        252,
        52,
        166,
        250,
        137,
        169
      ],
      "accounts": [
        {
          "name": "admin",
          "accounts": [
            {
              "name": "program",
              "address": "98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME"
            },
            {
              "name": "programData"
            },
            {
              "name": "authority",
              "writable": true,
              "signer": true
            }
          ]
        },
        {
          "name": "config",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "bridge",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  98,
                  114,
                  105,
                  100,
                  103,
                  101
                ]
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "policy",
          "type": {
            "defined": {
              "name": "mintPolicy"
            }
          }
        }
      ]
    },
    {
      "name": "setPauseGuardian",
      "docs": [
        "Names, rotates or clears (`None`) the pause guardian.",
        "",
        "Caller: the upgrade authority. It pays rent on the first call.",
        "Event: `PauseGuardianSet`.",
        "Errors: `InvalidAuthority`."
      ],
      "discriminator": [
        192,
        82,
        193,
        216,
        26,
        22,
        10,
        117
      ],
      "accounts": [
        {
          "name": "admin",
          "accounts": [
            {
              "name": "program",
              "address": "98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME"
            },
            {
              "name": "programData"
            },
            {
              "name": "authority",
              "writable": true,
              "signer": true
            }
          ]
        },
        {
          "name": "pauseGuardian",
          "docs": [
            "`init_if_needed`: the first call creates the account and later calls",
            "overwrite it. Reinitialization is harmless here: only the upgrade",
            "authority reaches the handler, the handler rewrites every field, the",
            "address is the canonical PDA and nothing closes the account."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  103,
                  117,
                  97,
                  114,
                  100,
                  105,
                  97,
                  110
                ]
              }
            ]
          }
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "guardian",
          "type": {
            "option": "pubkey"
          }
        }
      ]
    },
    {
      "name": "setTokenMetadata",
      "docs": [
        "Creates or updates the AKT mint's Metaplex metadata. Supply and token",
        "authorities do not change.",
        "",
        "Caller: the upgrade authority. It pays rent on creation.",
        "Event: `TokenMetadataSet`.",
        "Errors: `InvalidAuthority`, `InvalidMetadataUri`, `InvalidMetadata`."
      ],
      "discriminator": [
        218,
        126,
        122,
        193,
        220,
        149,
        103,
        39
      ],
      "accounts": [
        {
          "name": "admin",
          "accounts": [
            {
              "name": "program",
              "address": "98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME"
            },
            {
              "name": "programData"
            },
            {
              "name": "authority",
              "writable": true,
              "signer": true
            }
          ]
        },
        {
          "name": "config",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "mint",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  105,
                  110,
                  116
                ]
              }
            ]
          },
          "relations": [
            "config"
          ]
        },
        {
          "name": "mintAuthority",
          "docs": [
            "Metaplex CPI as mint authority and is the metadata update authority."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  105,
                  110,
                  116,
                  95,
                  97,
                  117,
                  116,
                  104,
                  111,
                  114,
                  105,
                  116,
                  121
                ]
              }
            ]
          }
        },
        {
          "name": "metadata",
          "docs": [
            "the handler checks its owner, mint and update authority first."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  101,
                  116,
                  97,
                  100,
                  97,
                  116,
                  97
                ]
              },
              {
                "kind": "account",
                "path": "metadataProgram"
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ],
            "program": {
              "kind": "account",
              "path": "metadataProgram"
            }
          }
        },
        {
          "name": "metadataProgram",
          "address": "metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s"
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "rent",
          "docs": [
            "Part of Metaplex's `CreateMetadataAccountsV3` account list."
          ],
          "address": "SysvarRent111111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "uri",
          "type": "string"
        }
      ]
    },
    {
      "name": "voidTransfer",
      "docs": [
        "Resolves an x/sink record with a permanent governance authorization in",
        "the native settlement at `settlement_status`. Works while minting is disabled.",
        "",
        "Caller: anyone, in a top-level instruction preceded only by",
        "ComputeBudget instructions. It pays rent for the status.",
        "Event: `TransferResolved`.",
        "Errors: `CpiNotAllowed`, `PrecedingInstruction`, `InvalidRecord`, the",
        "light-client and proof errors, and membership verification errors. A",
        "resolved record fails because its status exists."
      ],
      "discriminator": [
        242,
        219,
        72,
        159,
        81,
        185,
        100,
        23
      ],
      "accounts": [
        {
          "name": "payer",
          "docs": [
            "Pays rent for the status account."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "bridge",
          "docs": [
            "Pinned client. `bridge_enabled` is not read: governance voids work while",
            "minting is paused."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  98,
                  114,
                  105,
                  100,
                  103,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "proofBuffer",
          "docs": [
            "A complete proof of the settlement key of `id`, from any submitter's buffer."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  112,
                  114,
                  111,
                  111,
                  102
                ]
              },
              {
                "kind": "account",
                "path": "proof_buffer.submitter",
                "account": "proofBuffer"
              },
              {
                "kind": "account",
                "path": "proof_buffer.nonce",
                "account": "proofBuffer"
              }
            ]
          }
        },
        {
          "name": "status",
          "docs": [
            "Terminal status. `init` fails if the record already has one."
          ],
          "writable": true
        },
        {
          "name": "lightClientProgram"
        },
        {
          "name": "clientState",
          "docs": [
            "chain ID, frozen height and trusting period."
          ]
        },
        {
          "name": "latestConsensus",
          "docs": [
            "(the client's latest) and that it is inside the trusting period."
          ]
        },
        {
          "name": "proofConsensus",
          "docs": [
            "(`proof_height`); the pinned library verifies against its root."
          ]
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "id",
          "type": {
            "defined": {
              "name": "transferId"
            }
          }
        },
        {
          "name": "settlementStatus",
          "type": "u8"
        },
        {
          "name": "proofHeight",
          "type": "u64"
        }
      ]
    }
  ],
  "accounts": [
    {
      "name": "bridgeConfig",
      "discriminator": [
        40,
        206,
        51,
        233,
        246,
        40,
        178,
        85
      ]
    },
    {
      "name": "launchConfig",
      "discriminator": [
        18,
        161,
        9,
        224,
        102,
        145,
        29,
        94
      ]
    },
    {
      "name": "pauseGuardian",
      "discriminator": [
        198,
        150,
        21,
        87,
        129,
        78,
        135,
        183
      ]
    },
    {
      "name": "proofBuffer",
      "discriminator": [
        71,
        133,
        225,
        94,
        9,
        130,
        40,
        161
      ]
    },
    {
      "name": "transferStatus",
      "discriminator": [
        32,
        200,
        30,
        253,
        33,
        153,
        127,
        45
      ]
    }
  ],
  "events": [
    {
      "name": "bridgeConfigured",
      "discriminator": [
        123,
        128,
        67,
        78,
        101,
        146,
        43,
        25
      ]
    },
    {
      "name": "bridgeEnabledSet",
      "discriminator": [
        184,
        148,
        37,
        83,
        66,
        237,
        7,
        118
      ]
    },
    {
      "name": "guardianPaused",
      "discriminator": [
        170,
        43,
        221,
        225,
        225,
        137,
        249,
        184
      ]
    },
    {
      "name": "launchInitialized",
      "discriminator": [
        60,
        143,
        196,
        55,
        214,
        166,
        10,
        63
      ]
    },
    {
      "name": "lightClientRepinned",
      "discriminator": [
        150,
        163,
        24,
        40,
        214,
        71,
        171,
        186
      ]
    },
    {
      "name": "mintPolicySet",
      "discriminator": [
        194,
        195,
        143,
        171,
        251,
        203,
        243,
        211
      ]
    },
    {
      "name": "pauseGuardianSet",
      "discriminator": [
        65,
        50,
        111,
        132,
        193,
        5,
        19,
        69
      ]
    },
    {
      "name": "tokenMetadataSet",
      "discriminator": [
        203,
        73,
        184,
        127,
        183,
        146,
        2,
        247
      ]
    },
    {
      "name": "transferResolved",
      "discriminator": [
        205,
        27,
        228,
        215,
        90,
        3,
        236,
        176
      ]
    }
  ],
  "errors": [
    {
      "code": 7000,
      "name": "invalidProgramData",
      "msg": "ProgramData account does not belong to this program"
    },
    {
      "code": 7001,
      "name": "unauthorizedInitializer",
      "msg": "Only this program's upgrade authority may run genesis"
    },
    {
      "code": 7002,
      "name": "invalidAuthority",
      "msg": "Signer is not this program's upgrade authority"
    },
    {
      "code": 7003,
      "name": "unauthorizedGuardian",
      "msg": "Signer is not the configured pause guardian"
    },
    {
      "code": 7004,
      "name": "invalidMetadataUri",
      "msg": "Metadata URI must start with https:// and be 9 to 200 printable ASCII bytes"
    },
    {
      "code": 7005,
      "name": "invalidMetadata",
      "msg": "Existing metadata is not for the AKT mint or not controlled by the mint-authority PDA"
    },
    {
      "code": 7006,
      "name": "invalidChainId",
      "msg": "Chain ID must be 1 to 64 bytes"
    },
    {
      "code": 7007,
      "name": "invalidPolicy",
      "msg": "Mint policy needs 0 < per-transfer max <= daily cap and at most 16 nonzero vaults"
    },
    {
      "code": 7008,
      "name": "invalidLightClient",
      "msg": "Light-client account has the wrong owner or address, or invalid parameters"
    },
    {
      "code": 7009,
      "name": "wrongChain",
      "msg": "Light-client chain ID differs from the expected chain ID"
    },
    {
      "code": 7010,
      "name": "clientFrozen",
      "msg": "Light client is frozen"
    },
    {
      "code": 7011,
      "name": "clientExpired",
      "msg": "Light client's latest consensus state is older than its trusting period"
    },
    {
      "code": 7012,
      "name": "invalidConsensusTimestamp",
      "msg": "Latest consensus timestamp is zero or later than the clock plus the allowed drift"
    },
    {
      "code": 7013,
      "name": "invalidProofHeight",
      "msg": "Proof height must be between 1 and the client's latest height"
    },
    {
      "code": 7014,
      "name": "invalidProofLength",
      "msg": "Proof must declare 1 to 16384 bytes; chunks must be 1 to 500 bytes and fit that length"
    },
    {
      "code": 7015,
      "name": "invalidProofOffset",
      "msg": "Proof chunk offset must equal the number of bytes already written"
    },
    {
      "code": 7016,
      "name": "incompleteProof",
      "msg": "Proof buffer holds fewer bytes than it declared"
    },
    {
      "code": 7017,
      "name": "bridgeDisabled",
      "msg": "Bridge minting is disabled"
    },
    {
      "code": 7018,
      "name": "cpiNotAllowed",
      "msg": "Mint and void must be top-level instructions, not CPI"
    },
    {
      "code": 7019,
      "name": "precedingInstruction",
      "msg": "Only compute budget instructions may precede mint or void"
    },
    {
      "code": 7020,
      "name": "invalidRecord",
      "msg": "Record needs a 1-to-255-byte owner, positive sequence and amount, and 0 < height <= MaxInt64 with height < proof height"
    },
    {
      "code": 7021,
      "name": "invalidClock",
      "msg": "Solana clock is negative or earlier than the recorded mint day"
    },
    {
      "code": 7022,
      "name": "transferLimit",
      "msg": "Amount exceeds the per-transfer mint limit; retry after a policy change"
    },
    {
      "code": 7023,
      "name": "dailyLimit",
      "msg": "Amount would exceed the daily mint cap; retry on a later UTC day"
    },
    {
      "code": 7024,
      "name": "invalidProofNonce",
      "msg": "Proof buffer nonce must be the full SHA-256 of the proven key"
    },
    {
      "code": 7025,
      "name": "invalidSettlementStatus",
      "msg": "Canonical settlement status must be 1 through 5"
    },
    {
      "code": 7026,
      "name": "invalidVoidAuthorization",
      "msg": "Settlement must contain a unique, bounded permanent void authorization"
    },
    {
      "code": 7027,
      "name": "invalidMembershipProof",
      "msg": "Full IAVL and multistore membership proof does not verify"
    },
    {
      "code": 7028,
      "name": "proofBufferTooSmall",
      "msg": "Grow the proof buffer before appending beyond its allocation"
    }
  ],
  "types": [
    {
      "name": "bridgeConfig",
      "docs": [
        "Bridge settings at `[\"bridge\"]`, created once by `configure_bridge`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "lightClientProgram",
            "docs": [
              "ICS07 deployment whose verified state authorizes mints and voids.",
              "Only `repin_light_client` changes it."
            ],
            "type": "pubkey"
          },
          {
            "name": "chainId",
            "docs": [
              "Akash chain ID the pinned client must track."
            ],
            "type": "string"
          },
          {
            "name": "policy",
            "docs": [
              "Mint limits and vault allowlist."
            ],
            "type": {
              "defined": {
                "name": "mintPolicy"
              }
            }
          },
          {
            "name": "mintDay",
            "docs": [
              "UTC day number (Solana unix time / 86,400) of `minted_today`."
            ],
            "type": "u64"
          },
          {
            "name": "mintedToday",
            "docs": [
              "Base units minted by `mint_transfer` on `mint_day`."
            ],
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "bridgeConfigured",
      "docs": [
        "`configure_bridge` pinned the light client and set the first mint policy.",
        "Minting stays disabled."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "lightClientProgram",
            "type": "pubkey"
          },
          {
            "name": "chainId",
            "type": "string"
          },
          {
            "name": "policy",
            "type": {
              "defined": {
                "name": "mintPolicy"
              }
            }
          }
        ]
      }
    },
    {
      "name": "bridgeEnabledSet",
      "docs": [
        "The upgrade authority set `LaunchConfig::bridge_enabled`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "enabled",
            "type": "bool"
          },
          {
            "name": "authority",
            "docs": [
              "The signing upgrade authority."
            ],
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "guardianPaused",
      "docs": [
        "The pause guardian disabled bridge minting."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "guardian",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "launchConfig",
      "docs": [
        "Launch record at `[\"config\"]`. `initialize` creates it once and nothing",
        "closes it, so genesis cannot run again."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "version",
            "docs": [
              "Layout version, `LAUNCH_CONFIG_VERSION`."
            ],
            "type": "u8"
          },
          {
            "name": "initializer",
            "docs": [
              "Upgrade authority that ran genesis."
            ],
            "type": "pubkey"
          },
          {
            "name": "mint",
            "docs": [
              "The AKT mint at `[\"mint\"]`."
            ],
            "type": "pubkey"
          },
          {
            "name": "bridgeEnabled",
            "docs": [
              "Whether `mint_transfer` may run. Only the program upgrade authority can",
              "enable it; the upgrade authority or the pause guardian can disable it."
            ],
            "type": "bool"
          },
          {
            "name": "initializedAtSlot",
            "docs": [
              "Slot of genesis."
            ],
            "type": "u64"
          },
          {
            "name": "bump",
            "docs": [
              "Bump of `[\"config\"]`."
            ],
            "type": "u8"
          },
          {
            "name": "mintAuthorityBump",
            "docs": [
              "Bump of `[\"mint_authority\"]`, used to sign mint and metadata CPIs."
            ],
            "type": "u8"
          }
        ]
      }
    },
    {
      "name": "launchInitialized",
      "docs": [
        "`initialize` created the launch record and the AKT mint with zero supply."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "initializer",
            "docs": [
              "The upgrade authority that ran genesis."
            ],
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "lightClientRepinned",
      "docs": [
        "`repin_light_client` moved the pin. Policy and usage carry over."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "previousProgram",
            "type": "pubkey"
          },
          {
            "name": "program",
            "type": "pubkey"
          },
          {
            "name": "previousChainId",
            "type": "string"
          },
          {
            "name": "chainId",
            "type": "string"
          }
        ]
      }
    },
    {
      "name": "mintPolicy",
      "docs": [
        "Limits on bridge issuance, set by the upgrade authority."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "perTransferMax",
            "docs": [
              "Largest amount one record may mint, in base units."
            ],
            "type": "u64"
          },
          {
            "name": "dailyMintCap",
            "docs": [
              "Most base units all records together may mint per UTC day."
            ],
            "type": "u64"
          },
          {
            "name": "vaults",
            "docs": [
              "Recipients accepted although they are off-curve or not owned by the",
              "System Program, such as vault PDAs."
            ],
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "mintPolicySet",
      "docs": [
        "`set_mint_policy` replaced the policy. The day's usage carries over."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "previous",
            "type": {
              "defined": {
                "name": "mintPolicy"
              }
            }
          },
          {
            "name": "policy",
            "type": {
              "defined": {
                "name": "mintPolicy"
              }
            }
          }
        ]
      }
    },
    {
      "name": "pauseGuardian",
      "docs": [
        "Pause guardian at `[\"guardian\"]`. The guardian can pause minting without",
        "waiting out the upgrade authority's time lock. It cannot enable minting or",
        "change anything else."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "guardian",
            "docs": [
              "Key allowed to call `guardian_pause`; `None` allows no one."
            ],
            "type": {
              "option": "pubkey"
            }
          },
          {
            "name": "bump",
            "docs": [
              "Bump of `[\"guardian\"]`."
            ],
            "type": "u8"
          }
        ]
      }
    },
    {
      "name": "pauseGuardianSet",
      "docs": [
        "`set_pause_guardian` named, rotated or cleared (`None`) the guardian."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "previous",
            "type": {
              "option": "pubkey"
            }
          },
          {
            "name": "guardian",
            "type": {
              "option": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "proofBuffer",
      "docs": [
        "Upload buffer for one ICS23 proof at `[\"proof\", submitter, nonce]`. Only the",
        "submitter writes or closes it; any mint or void may read a complete one."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "submitter",
            "docs": [
              "Creator. It paid the rent and receives it back on close."
            ],
            "type": "pubkey"
          },
          {
            "name": "nonce",
            "docs": [
              "Full SHA-256 of the sink key this buffer proves."
            ],
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "expectedLen",
            "docs": [
              "Proof length declared at creation, 1 to `MAX_PROOF_BYTES`."
            ],
            "type": "u32"
          },
          {
            "name": "data",
            "docs": [
              "Bytes written so far, in order."
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "tokenMetadataSet",
      "docs": [
        "`set_token_metadata` created or updated the mint's Metaplex metadata."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "metadata",
            "type": "pubkey"
          },
          {
            "name": "uri",
            "type": "string"
          }
        ]
      }
    },
    {
      "name": "transferId",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "owner",
            "type": "bytes"
          },
          {
            "name": "height",
            "type": "u64"
          },
          {
            "name": "sequence",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "transferOutcome",
      "docs": [
        "How a record resolved."
      ],
      "type": {
        "kind": "enum",
        "variants": [
          {
            "name": "minted",
            "fields": [
              {
                "name": "amount",
                "type": "u64"
              },
              {
                "name": "recipient",
                "type": "pubkey"
              }
            ]
          },
          {
            "name": "voided",
            "fields": [
              {
                "name": "reason",
                "type": {
                  "defined": {
                    "name": "voidReason"
                  }
                }
              }
            ]
          }
        ]
      }
    },
    {
      "name": "transferResolved",
      "docs": [
        "A record reached its terminal status at `[\"transfer\", SHA256(owner), LE64(height), LE64(sequence)]`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "id",
            "type": {
              "defined": {
                "name": "transferId"
              }
            }
          },
          {
            "name": "proofHeight",
            "type": "u64"
          },
          {
            "name": "resolvedAtSlot",
            "type": "u64"
          },
          {
            "name": "outcome",
            "type": {
              "defined": {
                "name": "transferOutcome"
              }
            }
          }
        ]
      }
    },
    {
      "name": "transferStatus",
      "docs": [
        "Terminal outcome at `[\"transfer\", SHA256(owner), LE64(height), LE64(sequence)]`.",
        "`mint_transfer` and `void_transfer` both create it with `init`, and nothing",
        "closes it, so each record ID resolves exactly once."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "id",
            "docs": [
              "x/sink record ID."
            ],
            "type": {
              "defined": {
                "name": "transferId"
              }
            }
          },
          {
            "name": "proofHeight",
            "docs": [
              "Verified Akash height whose state proved the record or void order."
            ],
            "type": "u64"
          },
          {
            "name": "resolvedAtSlot",
            "docs": [
              "Slot of resolution. Reporters read this slot's block for the event."
            ],
            "type": "u64"
          },
          {
            "name": "outcome",
            "type": {
              "defined": {
                "name": "transferOutcome"
              }
            }
          }
        ]
      }
    },
    {
      "name": "voidReason",
      "docs": [
        "Why a record was voided."
      ],
      "type": {
        "kind": "enum",
        "variants": [
          {
            "name": "invalidRecipient"
          },
          {
            "name": "governance"
          }
        ]
      }
    }
  ]
};
