/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/ics07_tendermint.json`.
 */
export type Ics07Tendermint = {
  "address": "2dTK4NwEkv3gyY5dZnG4TuDYFMoNYQYrBqEGwTSmEmKr",
  "metadata": {
    "name": "ics07Tendermint",
    "version": "0.1.0",
    "spec": "0.1.0",
    "description": "ICS-07 Tendermint light client for IBC on Solana",
    "repository": "https://github.com/cosmos/solidity-ibc-eureka"
  },
  "instructions": [
    {
      "name": "acceptAccessManagerTransfer",
      "discriminator": [
        61,
        185,
        143,
        64,
        16,
        88,
        106,
        185
      ],
      "accounts": [
        {
          "name": "appState",
          "docs": [
            "PDA holding program-level settings; the `access_manager` field is updated on success."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "newAmState",
          "docs": [
            "Proposed access-manager state PDA derived from the pending program ID."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  99,
                  99,
                  101,
                  115,
                  115,
                  95,
                  109,
                  97,
                  110,
                  97,
                  103,
                  101,
                  114
                ]
              }
            ],
            "program": {
              "kind": "account",
              "path": "app_state.am_state.pending_access_manager",
              "account": "appState"
            }
          }
        },
        {
          "name": "admin",
          "docs": [
            "Admin signer authorized on the **new** access manager."
          ],
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar used by the access manager to inspect the transaction."
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": []
    },
    {
      "name": "assembleAndSubmitMisbehaviour",
      "docs": [
        "Assemble chunks and submit misbehaviour",
        "Automatically freezes the client and cleans up all chunks"
      ],
      "discriminator": [
        182,
        151,
        225,
        14,
        100,
        151,
        177,
        234
      ],
      "accounts": [
        {
          "name": "clientState",
          "docs": [
            "PDA holding the light client configuration; frozen when misbehaviour is confirmed."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  108,
                  105,
                  101,
                  110,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "appState",
          "docs": [
            "PDA holding program-level settings; provides the `access_manager` address for role checks."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "accessManager",
          "docs": [
            "Access-manager PDA used to verify the submitter holds the relayer role."
          ]
        },
        {
          "name": "trustedConsensusState1",
          "docs": [
            "First trusted consensus state referenced by the misbehaviour evidence."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  115,
                  101,
                  110,
                  115,
                  117,
                  115,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "trustedHeight1"
              }
            ]
          }
        },
        {
          "name": "trustedConsensusState2",
          "docs": [
            "Second trusted consensus state referenced by the misbehaviour evidence."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  115,
                  101,
                  110,
                  115,
                  117,
                  115,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "trustedHeight2"
              }
            ]
          }
        },
        {
          "name": "submitter",
          "docs": [
            "Relayer that uploaded the chunks, signs the assembly transaction and receives rent refunds."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar used by the access manager to inspect the transaction."
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "chunkCount",
          "type": "u8"
        },
        {
          "name": "trustedHeight1",
          "type": "u64"
        },
        {
          "name": "trustedHeight2",
          "type": "u64"
        }
      ]
    },
    {
      "name": "assembleAndUpdateClient",
      "docs": [
        "Assemble chunks and update the client",
        "Automatically cleans up all chunks after successful update"
      ],
      "discriminator": [
        86,
        215,
        199,
        79,
        131,
        79,
        180,
        158
      ],
      "accounts": [
        {
          "name": "clientState",
          "docs": [
            "PDA holding the light client configuration; updated with the new latest height on success."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  108,
                  105,
                  101,
                  110,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "appState",
          "docs": [
            "PDA holding program-level settings; provides the `access_manager` address for role checks."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "accessManager",
          "docs": [
            "Access-manager PDA used to verify the submitter holds the relayer role."
          ]
        },
        {
          "name": "trustedConsensusState",
          "docs": [
            "Consensus state the header declares as its trust anchor; validated against PDA seeds."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  115,
                  101,
                  110,
                  115,
                  117,
                  115,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "trustedHeight"
              }
            ]
          }
        },
        {
          "name": "newConsensusStateStore",
          "docs": [
            "Destination PDA for the newly derived consensus state; created if it does not already exist."
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
                  115,
                  101,
                  110,
                  115,
                  117,
                  115,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "targetHeight"
              }
            ]
          }
        },
        {
          "name": "submitter",
          "docs": [
            "Relayer that uploaded the chunks, signs the assembly transaction and receives rent refunds."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "systemProgram",
          "docs": [
            "Required by Anchor for creating the new consensus-state PDA."
          ],
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar used by the access manager to inspect the transaction."
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "targetHeight",
          "type": "u64"
        },
        {
          "name": "chunkCount",
          "type": "u8"
        },
        {
          "name": "trustedHeight",
          "type": "u64"
        }
      ],
      "returns": {
        "defined": {
          "name": "updateResult"
        }
      }
    },
    {
      "name": "cancelAccessManagerTransfer",
      "discriminator": [
        181,
        250,
        216,
        163,
        132,
        73,
        23,
        68
      ],
      "accounts": [
        {
          "name": "appState",
          "docs": [
            "PDA holding program-level settings; the pending transfer is cleared on success."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "amState",
          "docs": [
            "Current access-manager state PDA used to verify the caller holds the admin role."
          ]
        },
        {
          "name": "admin",
          "docs": [
            "Admin signer authorized to cancel the transfer."
          ],
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar used by the access manager to inspect the transaction."
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": []
    },
    {
      "name": "cleanupIncompleteMisbehaviour",
      "docs": [
        "Clean up incomplete misbehaviour uploads",
        "This can be called to reclaim rent from failed or abandoned misbehaviour submissions"
      ],
      "discriminator": [
        128,
        250,
        150,
        138,
        153,
        111,
        26,
        224
      ],
      "accounts": [
        {
          "name": "submitter",
          "docs": [
            "Original submitter who uploaded the chunks; must sign to prove ownership and receives rent refunds."
          ],
          "writable": true,
          "signer": true
        }
      ],
      "args": []
    },
    {
      "name": "cleanupIncompleteUpload",
      "docs": [
        "Clean up incomplete header uploads",
        "This can be called to reclaim rent from failed or abandoned uploads",
        "Closes both `HeaderChunk` and `SignatureVerification` PDAs owned by the submitter"
      ],
      "discriminator": [
        53,
        54,
        142,
        122,
        91,
        50,
        60,
        171
      ],
      "accounts": [
        {
          "name": "submitter",
          "docs": [
            "Original submitter who uploaded the chunks; must sign to prove ownership and receives rent refunds."
          ],
          "writable": true,
          "signer": true
        }
      ],
      "args": []
    },
    {
      "name": "clientStatus",
      "discriminator": [
        75,
        93,
        126,
        21,
        10,
        112,
        109,
        139
      ],
      "accounts": [
        {
          "name": "clientState",
          "docs": [
            "PDA holding the light client configuration; checked for frozen state and trusting period."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  108,
                  105,
                  101,
                  110,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "consensusState",
          "docs": [
            "Consensus state at the latest height; its timestamp is compared against the",
            "current clock to determine whether the client has expired."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  115,
                  101,
                  110,
                  115,
                  117,
                  115,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "account",
                "path": "client_state.latest_height.revision_height",
                "account": "clientState"
              }
            ]
          }
        }
      ],
      "args": []
    },
    {
      "name": "initialize",
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
          "name": "clientStateAccount",
          "docs": [
            "PDA holding the Tendermint light client configuration and tracking state."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  108,
                  105,
                  101,
                  110,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "consensusStateStore",
          "docs": [
            "PDA storing the verified consensus state at the initial height."
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
                  115,
                  101,
                  110,
                  115,
                  117,
                  115,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "client_state.latest_height.revision_height"
              }
            ]
          }
        },
        {
          "name": "appState",
          "docs": [
            "PDA holding program-level settings such as the `access_manager` address and `chain_id`."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "payer",
          "docs": [
            "Signer that pays for PDA account creation."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "systemProgram",
          "docs": [
            "Required by Anchor for PDA creation via the System Program."
          ],
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "programData",
          "docs": [
            "BPF Loader Upgradeable `ProgramData` account for this program."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  24,
                  50,
                  88,
                  173,
                  8,
                  80,
                  188,
                  92,
                  37,
                  31,
                  48,
                  82,
                  192,
                  222,
                  56,
                  110,
                  29,
                  250,
                  20,
                  121,
                  190,
                  159,
                  172,
                  123,
                  129,
                  11,
                  11,
                  206,
                  73,
                  73,
                  112,
                  93
                ]
              }
            ],
            "program": {
              "kind": "const",
              "value": [
                2,
                168,
                246,
                145,
                78,
                136,
                161,
                176,
                226,
                16,
                21,
                62,
                247,
                99,
                174,
                43,
                0,
                194,
                185,
                61,
                22,
                193,
                36,
                210,
                192,
                83,
                122,
                16,
                4,
                128,
                0,
                0
              ]
            }
          }
        },
        {
          "name": "authority",
          "docs": [
            "The program's upgrade authority — must sign to prove deployer identity."
          ],
          "signer": true
        }
      ],
      "args": [
        {
          "name": "clientState",
          "type": {
            "defined": {
              "name": "clientState"
            }
          }
        },
        {
          "name": "consensusState",
          "type": {
            "defined": {
              "name": "consensusState"
            }
          }
        },
        {
          "name": "accessManager",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "initializeSignatureBatch",
      "discriminator": [
        16,
        143,
        211,
        69,
        214,
        73,
        29,
        119
      ],
      "accounts": [
        {
          "name": "signatureBatch",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  105,
                  103,
                  95,
                  98,
                  97,
                  116,
                  99,
                  104
                ]
              },
              {
                "kind": "account",
                "path": "submitter"
              },
              {
                "kind": "arg",
                "path": "session"
              }
            ]
          }
        },
        {
          "name": "appState",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "accessManager"
        },
        {
          "name": "submitter",
          "writable": true,
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "address": "Sysvar1nstructions1111111111111111111111111"
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "session",
          "type": {
            "array": [
              "u8",
              32
            ]
          }
        }
      ]
    },
    {
      "name": "preVerifySignature",
      "discriminator": [
        121,
        29,
        68,
        190,
        85,
        124,
        111,
        45
      ],
      "accounts": [
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar used to load and introspect the preceding Ed25519 verify instruction."
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        },
        {
          "name": "signatureVerification",
          "docs": [
            "PDA that stores the Ed25519 verification outcome, keyed by the signature hash."
          ],
          "writable": true
        },
        {
          "name": "appState",
          "docs": [
            "PDA holding program-level settings; provides the `access_manager` address for role checks."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "accessManager",
          "docs": [
            "Access-manager PDA used to verify the submitter holds the relayer role."
          ]
        },
        {
          "name": "submitter",
          "docs": [
            "Relayer that signs the transaction and pays for the verification PDA creation."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "systemProgram",
          "docs": [
            "Required by Anchor for PDA creation via the System Program."
          ],
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "signature",
          "type": {
            "defined": {
              "name": "signatureData"
            }
          }
        }
      ]
    },
    {
      "name": "preVerifySignatureBatch",
      "discriminator": [
        78,
        220,
        160,
        155,
        226,
        154,
        148,
        254
      ],
      "accounts": [
        {
          "name": "signatureBatch",
          "writable": true
        },
        {
          "name": "appState",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "accessManager"
        },
        {
          "name": "submitter",
          "signer": true,
          "relations": [
            "signatureBatch"
          ]
        },
        {
          "name": "instructionsSysvar",
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "signature",
          "type": {
            "defined": {
              "name": "signatureData"
            }
          }
        }
      ]
    },
    {
      "name": "proposeAccessManagerTransfer",
      "discriminator": [
        96,
        76,
        46,
        161,
        93,
        100,
        32,
        145
      ],
      "accounts": [
        {
          "name": "appState",
          "docs": [
            "PDA holding program-level settings including the current `access_manager`."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "accessManager",
          "docs": [
            "Current access-manager state PDA used to verify the caller holds the admin role."
          ]
        },
        {
          "name": "admin",
          "docs": [
            "Admin signer authorized to propose the transfer."
          ],
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar used by the access manager to inspect the transaction."
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "newAccessManager",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "pruneConsensusState",
      "docs": [
        "Local AKT extension: close an expired consensus state other than the",
        "latest and return its rent to the relayer."
      ],
      "discriminator": [
        140,
        71,
        141,
        15,
        203,
        79,
        159,
        201
      ],
      "accounts": [
        {
          "name": "clientState",
          "docs": [
            "Supplies the latest height and trusting period."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  108,
                  105,
                  101,
                  110,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "appState",
          "docs": [
            "Provides the `access_manager` address for the role check."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "accessManager"
        },
        {
          "name": "consensusStateStore",
          "docs": [
            "The consensus state to close."
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
                  115,
                  101,
                  110,
                  115,
                  117,
                  115,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "height"
              }
            ]
          }
        },
        {
          "name": "submitter",
          "docs": [
            "Relayer holding `RELAYER_ROLE`; receives the reclaimed rent."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "height",
          "type": "u64"
        }
      ]
    },
    {
      "name": "uploadHeaderChunk",
      "docs": [
        "Upload a chunk of header data for multi-transaction updates",
        "Fails if a chunk already exists at this position (no overwrites allowed)"
      ],
      "discriminator": [
        154,
        38,
        82,
        143,
        56,
        2,
        24,
        33
      ],
      "accounts": [
        {
          "name": "chunk",
          "docs": [
            "PDA storing one segment of the serialized header, keyed by submitter, target height and chunk index."
          ],
          "writable": true
        },
        {
          "name": "clientState",
          "docs": [
            "PDA holding the light client configuration; used to check the frozen status."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  108,
                  105,
                  101,
                  110,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "appState",
          "docs": [
            "PDA holding program-level settings; provides the `access_manager` address for role checks."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "accessManager",
          "docs": [
            "Access-manager PDA used to verify the submitter holds the relayer role."
          ]
        },
        {
          "name": "submitter",
          "docs": [
            "Relayer that signs the transaction and pays for chunk account creation."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar used by the access manager to inspect the transaction."
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        },
        {
          "name": "systemProgram",
          "docs": [
            "Required by Anchor for PDA creation via the System Program."
          ],
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "uploadChunkParams"
            }
          }
        }
      ]
    },
    {
      "name": "uploadMisbehaviourChunk",
      "docs": [
        "Upload a chunk of misbehaviour data for multi-transaction submission"
      ],
      "discriminator": [
        150,
        89,
        208,
        203,
        92,
        179,
        160,
        140
      ],
      "accounts": [
        {
          "name": "chunk",
          "docs": [
            "PDA storing one segment of the serialized misbehaviour, keyed by submitter and chunk index."
          ],
          "writable": true
        },
        {
          "name": "clientState",
          "docs": [
            "PDA holding the light client configuration; used to check the frozen status."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  108,
                  105,
                  101,
                  110,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "appState",
          "docs": [
            "PDA holding program-level settings; provides the `access_manager` address for role checks."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  112,
                  112,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "accessManager",
          "docs": [
            "Access-manager PDA used to verify the submitter holds the relayer role."
          ]
        },
        {
          "name": "submitter",
          "docs": [
            "Relayer that signs the transaction and pays for chunk account creation."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar used by the access manager to inspect the transaction."
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        },
        {
          "name": "systemProgram",
          "docs": [
            "Required by Anchor for PDA creation via the System Program."
          ],
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "uploadMisbehaviourChunkParams"
            }
          }
        }
      ]
    },
    {
      "name": "verifyMembership",
      "discriminator": [
        101,
        53,
        78,
        0,
        103,
        151,
        236,
        209
      ],
      "accounts": [
        {
          "name": "clientState",
          "docs": [
            "PDA holding the light client configuration; used to check the frozen status."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  108,
                  105,
                  101,
                  110,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "consensusStateAtHeight",
          "docs": [
            "PDA storing the verified consensus state at the requested proof height."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  115,
                  101,
                  110,
                  115,
                  117,
                  115,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "msg.height"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "msg",
          "type": {
            "defined": {
              "name": "membershipMsg"
            }
          }
        }
      ]
    },
    {
      "name": "verifyNonMembership",
      "docs": [
        "Verifies the absence of a value at a given path in the counterparty chain state.",
        "Returns the timestamp of the consensus state at the proof height in unix seconds."
      ],
      "discriminator": [
        231,
        161,
        86,
        239,
        111,
        236,
        14,
        74
      ],
      "accounts": [
        {
          "name": "clientState",
          "docs": [
            "PDA holding the light client configuration; used to check the frozen status."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  108,
                  105,
                  101,
                  110,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "consensusStateAtHeight",
          "docs": [
            "PDA storing the verified consensus state at the requested proof height."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  115,
                  101,
                  110,
                  115,
                  117,
                  115,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "msg.height"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "msg",
          "type": {
            "defined": {
              "name": "nonMembershipMsg"
            }
          }
        }
      ]
    }
  ],
  "accounts": [
    {
      "name": "appState",
      "discriminator": [
        217,
        117,
        146,
        200,
        12,
        223,
        18,
        55
      ]
    },
    {
      "name": "clientState",
      "discriminator": [
        147,
        10,
        249,
        80,
        145,
        124,
        219,
        60
      ]
    },
    {
      "name": "consensusStateStore",
      "discriminator": [
        82,
        126,
        130,
        187,
        68,
        64,
        80,
        32
      ]
    },
    {
      "name": "headerChunk",
      "discriminator": [
        236,
        15,
        220,
        133,
        128,
        6,
        145,
        240
      ]
    },
    {
      "name": "misbehaviourChunk",
      "discriminator": [
        176,
        75,
        200,
        147,
        222,
        20,
        68,
        192
      ]
    },
    {
      "name": "signatureBatch",
      "discriminator": [
        58,
        220,
        101,
        246,
        79,
        152,
        151,
        56
      ]
    },
    {
      "name": "signatureVerification",
      "discriminator": [
        81,
        117,
        47,
        112,
        35,
        80,
        229,
        151
      ]
    }
  ],
  "errors": [
    {
      "code": 6000,
      "name": "clientFrozen",
      "msg": "Client is frozen"
    },
    {
      "code": 6001,
      "name": "clientAlreadyFrozen",
      "msg": "Client is already frozen"
    },
    {
      "code": 6002,
      "name": "invalidChainId",
      "msg": "Invalid chain ID: cannot be empty"
    },
    {
      "code": 6003,
      "name": "invalidTrustLevel",
      "msg": "Invalid trust level: numerator must be > 0 and <= denominator"
    },
    {
      "code": 6004,
      "name": "invalidPeriods",
      "msg": "Invalid periods: trusting period must be positive and less than unbonding period"
    },
    {
      "code": 6005,
      "name": "invalidMaxClockDrift",
      "msg": "Invalid max clock drift: must be positive"
    },
    {
      "code": 6006,
      "name": "invalidHeight",
      "msg": "Invalid height: height cannot be zero"
    },
    {
      "code": 6007,
      "name": "heightMismatch",
      "msg": "Height mismatch: expected height does not match provided height"
    },
    {
      "code": 6008,
      "name": "invalidHeader",
      "msg": "Invalid header: failed to deserialize or validate header"
    },
    {
      "code": 6009,
      "name": "invalidProof",
      "msg": "Invalid proof: proof validation failed"
    },
    {
      "code": 6010,
      "name": "updateClientFailed",
      "msg": "Update client failed: header verification failed"
    },
    {
      "code": 6011,
      "name": "invalidMisbehaviour",
      "msg": "Invalid misbehaviour: failed to deserialize or validate misbehaviour"
    },
    {
      "code": 6012,
      "name": "misbehaviourConflictingState",
      "msg": "Misbehaviour detected: conflicting consensus state at same height"
    },
    {
      "code": 6013,
      "name": "misbehaviourNonIncreasingTime",
      "msg": "Misbehaviour detected: non-increasing timestamp"
    },
    {
      "code": 6014,
      "name": "misbehaviourCheckFailed",
      "msg": "Misbehaviour check failed: invalid misbehaviour proof"
    },
    {
      "code": 6015,
      "name": "membershipVerificationFailed",
      "msg": "Membership verification failed: proof does not match commitment"
    },
    {
      "code": 6016,
      "name": "membershipEmptyValue",
      "msg": "Membership verification failed: value cannot be empty"
    },
    {
      "code": 6017,
      "name": "nonMembershipVerificationFailed",
      "msg": "Non-membership verification failed: key exists when it should not"
    },
    {
      "code": 6018,
      "name": "invalidValue",
      "msg": "Invalid value: non-membership proof must have empty value"
    },
    {
      "code": 6019,
      "name": "nonZeroDelay",
      "msg": "Time and block delay must be zero"
    },
    {
      "code": 6020,
      "name": "consensusStateNotFound",
      "msg": "Consensus state not found at the specified height"
    },
    {
      "code": 6021,
      "name": "invalidChunkCount",
      "msg": "Invalid chunk count: unexpected number of chunk accounts"
    },
    {
      "code": 6022,
      "name": "invalidChunkAccount",
      "msg": "Invalid chunk account: chunk account PDA mismatch"
    },
    {
      "code": 6023,
      "name": "chunkDataTooLarge",
      "msg": "Chunk data too large: exceeds maximum chunk size"
    },
    {
      "code": 6024,
      "name": "unauthorizedRole",
      "msg": "Unauthorized: caller does not have required role"
    },
    {
      "code": 6025,
      "name": "invalidNumberOfAccounts",
      "msg": "Invalid number of accounts: mismatch between signatures and PDAs"
    },
    {
      "code": 6026,
      "name": "invalidAccountData",
      "msg": "Invalid account data: account data is malformed or too small"
    },
    {
      "code": 6027,
      "name": "serializationError",
      "msg": "Serialization error: failed to serialize/deserialize data"
    },
    {
      "code": 6028,
      "name": "accountValidationFailed",
      "msg": "Account validation failed: invalid account or PDA"
    },
    {
      "code": 6029,
      "name": "invalidAccountOwner",
      "msg": "Invalid account owner: account is not owned by the expected program"
    },
    {
      "code": 6030,
      "name": "arithmeticOverflow",
      "msg": "Arithmetic overflow detected"
    },
    {
      "code": 6031,
      "name": "conversionError",
      "msg": "Conversion error: invalid header data"
    },
    {
      "code": 6032,
      "name": "invalidAccessManager",
      "msg": "Invalid access manager"
    },
    {
      "code": 6033,
      "name": "unauthorizedDeployer",
      "msg": "Only the program's upgrade authority can call initialize"
    },
    {
      "code": 6034,
      "name": "cannotPruneLatestConsensusState",
      "msg": "Consensus state at the latest height cannot be pruned"
    },
    {
      "code": 6035,
      "name": "consensusStateNotExpired",
      "msg": "Consensus state is still within the trusting period"
    }
  ],
  "types": [
    {
      "name": "accessManagerState",
      "docs": [
        "Embedded access manager state for IBC programs.",
        "",
        "Each IBC program embeds this struct in its on-chain state account to track",
        "which access manager program governs its permissioned instructions and to",
        "support two-step access manager migration (propose/accept).",
        "",
        "Does not carry its own `_reserved` field — future fields can eat into the",
        "`_reserved` space of the higher-level state that embeds this struct."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "accessManager",
            "docs": [
              "Program ID of the access manager that governs this program's roles."
            ],
            "type": "pubkey"
          },
          {
            "name": "pendingAccessManager",
            "docs": [
              "Proposed replacement access manager, set during a pending transfer."
            ],
            "type": {
              "option": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "appState",
      "docs": [
        "Global ICS07 Tendermint program configuration.",
        "",
        "Singleton PDA that links the light client program to its access manager",
        "for admin-gated operations (e.g. `access_manager_transfer`) and stores the",
        "chain ID for introspection by off-chain tooling."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "amState",
            "docs": [
              "Access manager transfer state for two-step propose/accept"
            ],
            "type": {
              "defined": {
                "name": "accessManagerState"
              }
            }
          },
          {
            "name": "reserved",
            "docs": [
              "Reserved space for future fields"
            ],
            "type": {
              "array": [
                "u8",
                256
              ]
            }
          }
        ]
      }
    },
    {
      "name": "clientState",
      "docs": [
        "Tendermint light client configuration and tracking state.",
        "",
        "Holds the trust parameters (trust level, trusting/unbonding periods,",
        "max clock drift) and the latest verified height. The `update_client`",
        "instruction reads and updates this account when a new header is",
        "successfully verified. If misbehaviour is detected the client is frozen",
        "by setting `frozen_height`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainId",
            "type": "string"
          },
          {
            "name": "trustLevelNumerator",
            "type": "u64"
          },
          {
            "name": "trustLevelDenominator",
            "type": "u64"
          },
          {
            "name": "trustingPeriod",
            "type": "u64"
          },
          {
            "name": "unbondingPeriod",
            "type": "u64"
          },
          {
            "name": "maxClockDrift",
            "type": "u64"
          },
          {
            "name": "frozenHeight",
            "type": {
              "defined": {
                "name": "ibcHeight"
              }
            }
          },
          {
            "name": "latestHeight",
            "type": {
              "defined": {
                "name": "ibcHeight"
              }
            }
          }
        ]
      }
    },
    {
      "name": "consensusState",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "timestamp",
            "type": "u64"
          },
          {
            "name": "root",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "nextValidatorsHash",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          }
        ]
      }
    },
    {
      "name": "consensusStateStore",
      "docs": [
        "On-chain PDA storing a Tendermint consensus state for a specific block height.",
        "",
        "Contains the block timestamp, Merkle root and next-validators hash.",
        "The ICS26 router reads these accounts to verify IBC packet commitment",
        "proofs against the confirmed Tendermint state. One account exists per",
        "verified height, derived from `[\"consensus_state\", height_le_bytes]`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "height",
            "type": "u64"
          },
          {
            "name": "consensusState",
            "type": {
              "defined": {
                "name": "consensusState"
              }
            }
          }
        ]
      }
    },
    {
      "name": "headerChunk",
      "docs": [
        "Temporary storage for a single chunk of a Tendermint header during",
        "multi-transaction upload.",
        "",
        "Tendermint headers can exceed the Solana transaction size limit, so",
        "they are uploaded in chunks across multiple transactions. Once all",
        "chunks are present the `update_client` instruction reassembles and",
        "verifies the full header, then closes these accounts to reclaim rent."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "submitter",
            "docs": [
              "The submitter who created this chunk"
            ],
            "type": "pubkey"
          },
          {
            "name": "chunkData",
            "docs": [
              "The chunk data"
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "ibcHeight",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "revisionNumber",
            "type": "u64"
          },
          {
            "name": "revisionHeight",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "membershipMsg",
      "docs": [
        "Standard message structure for membership verification",
        "All light clients must accept this structure"
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "height",
            "docs": [
              "The height at which to verify"
            ],
            "type": "u64"
          },
          {
            "name": "proof",
            "docs": [
              "The merkle proof"
            ],
            "type": "bytes"
          },
          {
            "name": "path",
            "docs": [
              "The merkle path to the value"
            ],
            "type": {
              "vec": "bytes"
            }
          },
          {
            "name": "value",
            "docs": [
              "The value to verify"
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "misbehaviourChunk",
      "docs": [
        "Temporary storage for a single chunk of misbehaviour evidence during",
        "multi-transaction upload.",
        "",
        "Similar to [`HeaderChunk`] but used for misbehaviour reports that prove",
        "a validator set signed conflicting blocks. Once fully assembled, the",
        "`misbehaviour` instruction verifies the evidence and freezes the client."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chunkData",
            "docs": [
              "The chunk data"
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "nonMembershipMsg",
      "docs": [
        "Standard message structure for non-membership verification",
        "All light clients must accept this structure"
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "height",
            "docs": [
              "The height at which to verify"
            ],
            "type": "u64"
          },
          {
            "name": "proof",
            "docs": [
              "The merkle proof"
            ],
            "type": "bytes"
          },
          {
            "name": "path",
            "docs": [
              "The merkle path to the value"
            ],
            "type": {
              "vec": "bytes"
            }
          }
        ]
      }
    },
    {
      "name": "signatureBatch",
      "docs": [
        "Only this program's Ed25519-backed append instruction can add a digest."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "version",
            "type": "u8"
          },
          {
            "name": "submitter",
            "type": "pubkey"
          },
          {
            "name": "session",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "bump",
            "type": "u8"
          },
          {
            "name": "hashes",
            "type": {
              "vec": {
                "array": [
                  "u8",
                  32
                ]
              }
            }
          }
        ]
      }
    },
    {
      "name": "signatureData",
      "docs": [
        "Ed25519 signature data for pre-verification"
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "signatureHash",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "pubkey",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "msg",
            "type": "bytes"
          },
          {
            "name": "signature",
            "type": {
              "array": [
                "u8",
                64
              ]
            }
          }
        ]
      }
    },
    {
      "name": "signatureVerification",
      "docs": [
        "Stores the result of an Ed25519 signature verification.",
        "",
        "Because Solana's Ed25519 precompile writes results to a separate account",
        "rather than returning them inline, this account acts as a bridge:",
        "the precompile writes the verification outcome here, and the",
        "`update_client` instruction reads it to confirm header signatures.",
        "",
        "IMPORTANT: Field order matters — the verifier reads `data[8]` for `is_valid`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "isValid",
            "docs": [
              "Whether the signature is valid"
            ],
            "type": "bool"
          },
          {
            "name": "submitter",
            "docs": [
              "The submitter who created this verification"
            ],
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "updateResult",
      "type": {
        "kind": "enum",
        "variants": [
          {
            "name": "updateSuccess"
          },
          {
            "name": "noOp"
          },
          {
            "name": "misbehaviour"
          }
        ]
      }
    },
    {
      "name": "uploadChunkParams",
      "docs": [
        "Parameters for uploading a header chunk"
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "targetHeight",
            "type": "u64"
          },
          {
            "name": "chunkIndex",
            "type": "u8"
          },
          {
            "name": "chunkData",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "uploadMisbehaviourChunkParams",
      "docs": [
        "Parameters for uploading a misbehaviour chunk"
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chunkIndex",
            "type": "u8"
          },
          {
            "name": "chunkData",
            "type": "bytes"
          }
        ]
      }
    }
  ]
};
