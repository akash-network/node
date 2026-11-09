/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/access_manager.json`.
 */
export type AccessManager = {
  "address": "HntLc7xrcRMRk9X38uMytCnweFJhVVCuzqhkVPe1RkCq",
  "metadata": {
    "name": "accessManager",
    "version": "0.1.0",
    "spec": "0.1.0",
    "description": "Role-based access manager for IBC on Solana",
    "repository": "https://github.com/cosmos/solidity-ibc-eureka"
  },
  "instructions": [
    {
      "name": "acceptUpgradeAuthorityTransfer",
      "discriminator": [
        95,
        41,
        116,
        54,
        102,
        173,
        111,
        17
      ],
      "accounts": [
        {
          "name": "accessManager",
          "writable": true,
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
            ]
          }
        },
        {
          "name": "programData",
          "docs": [
            "The target program's data account (BPF Loader Upgradeable PDA)."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "arg",
                "path": "targetProgram"
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
          "name": "upgradeAuthority",
          "docs": [
            "`AccessManager`'s PDA that acts as the current upgrade authority."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  117,
                  112,
                  103,
                  114,
                  97,
                  100,
                  101,
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
              },
              {
                "kind": "arg",
                "path": "targetProgram"
              }
            ]
          }
        },
        {
          "name": "newAuthority",
          "docs": [
            "The proposed new upgrade authority. Must match the pending transfer."
          ],
          "signer": true
        },
        {
          "name": "bpfLoaderUpgradeable",
          "address": "BPFLoaderUpgradeab1e11111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "targetProgram",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "cancelUpgradeAuthorityTransfer",
      "discriminator": [
        114,
        97,
        59,
        64,
        250,
        180,
        192,
        135
      ],
      "accounts": [
        {
          "name": "accessManager",
          "writable": true,
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
            ]
          }
        },
        {
          "name": "admin",
          "docs": [
            "Must hold `ADMIN_ROLE` on the current access manager."
          ],
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "targetProgram",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "claimUpgradeAuthority",
      "discriminator": [
        102,
        75,
        224,
        204,
        8,
        193,
        89,
        137
      ],
      "accounts": [
        {
          "name": "newUpgradeAuthority",
          "docs": [
            "This access manager's upgrade authority PDA for the target program.",
            "Signs the CPI as the new authority."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  117,
                  112,
                  103,
                  114,
                  97,
                  100,
                  101,
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
              },
              {
                "kind": "arg",
                "path": "targetProgram"
              }
            ]
          }
        },
        {
          "name": "sourceAccessManagerState",
          "docs": [
            "The source access manager's state PDA."
          ],
          "writable": true,
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
              "path": "sourceAccessManagerProgram"
            }
          }
        },
        {
          "name": "targetProgramData",
          "docs": [
            "The target program's data account (BPF Loader Upgradeable PDA)."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "arg",
                "path": "targetProgram"
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
          "name": "sourceUpgradeAuthority",
          "docs": [
            "The source access manager's upgrade authority PDA for the target program."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  117,
                  112,
                  103,
                  114,
                  97,
                  100,
                  101,
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
              },
              {
                "kind": "arg",
                "path": "targetProgram"
              }
            ],
            "program": {
              "kind": "account",
              "path": "sourceAccessManagerProgram"
            }
          }
        },
        {
          "name": "sourceAccessManagerProgram",
          "docs": [
            "The source access manager program (CPI target)."
          ]
        },
        {
          "name": "bpfLoaderUpgradeable",
          "address": "BPFLoaderUpgradeab1e11111111111111111111111"
        },
        {
          "name": "amState",
          "docs": [
            "Access manager state PDA used to verify the caller holds the admin role."
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
            ]
          }
        },
        {
          "name": "admin",
          "docs": [
            "Must hold `ADMIN_ROLE` on the current access manager."
          ],
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "targetProgram",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "grantRole",
      "discriminator": [
        218,
        234,
        128,
        15,
        82,
        33,
        236,
        253
      ],
      "accounts": [
        {
          "name": "accessManager",
          "docs": [
            "The access manager PDA storing all role assignments."
          ],
          "writable": true,
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
            ]
          }
        },
        {
          "name": "admin",
          "docs": [
            "The admin signer authorizing this role grant."
          ],
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar for CPI validation"
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "roleId",
          "type": "u64"
        },
        {
          "name": "account",
          "type": "pubkey"
        }
      ]
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
          "name": "accessManager",
          "docs": [
            "The access manager PDA to initialize (created here)."
          ],
          "writable": true,
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
            ]
          }
        },
        {
          "name": "payer",
          "docs": [
            "Pays for account creation rent."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "systemProgram",
          "docs": [
            "Required for PDA account creation."
          ],
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar for CPI validation"
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
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
                  249,
                  124,
                  44,
                  225,
                  218,
                  213,
                  151,
                  24,
                  246,
                  111,
                  70,
                  61,
                  181,
                  12,
                  183,
                  250,
                  191,
                  155,
                  252,
                  90,
                  94,
                  248,
                  40,
                  17,
                  182,
                  222,
                  210,
                  252,
                  28,
                  153,
                  62,
                  26
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
          "name": "admin",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "proposeUpgradeAuthorityTransfer",
      "discriminator": [
        235,
        222,
        36,
        145,
        215,
        244,
        214,
        152
      ],
      "accounts": [
        {
          "name": "accessManager",
          "writable": true,
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
            ]
          }
        },
        {
          "name": "admin",
          "docs": [
            "Must hold `ADMIN_ROLE` on the current access manager."
          ],
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "targetProgram",
          "type": "pubkey"
        },
        {
          "name": "newAuthority",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "renounceRole",
      "discriminator": [
        21,
        253,
        233,
        162,
        186,
        216,
        217,
        26
      ],
      "accounts": [
        {
          "name": "accessManager",
          "docs": [
            "The access manager PDA storing all role assignments."
          ],
          "writable": true,
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
            ]
          }
        },
        {
          "name": "caller",
          "docs": [
            "The signer renouncing their own role."
          ],
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar for CPI validation"
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "roleId",
          "type": "u64"
        }
      ]
    },
    {
      "name": "revokeRole",
      "discriminator": [
        179,
        232,
        2,
        180,
        48,
        227,
        82,
        7
      ],
      "accounts": [
        {
          "name": "accessManager",
          "docs": [
            "The access manager PDA storing all role assignments."
          ],
          "writable": true,
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
            ]
          }
        },
        {
          "name": "admin",
          "docs": [
            "The admin signer authorizing this role revocation."
          ],
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar for CPI validation"
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "roleId",
          "type": "u64"
        },
        {
          "name": "account",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "setWhitelistedPrograms",
      "discriminator": [
        215,
        39,
        132,
        81,
        64,
        65,
        123,
        123
      ],
      "accounts": [
        {
          "name": "accessManager",
          "docs": [
            "The access manager PDA whose whitelist will be updated."
          ],
          "writable": true,
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
            ]
          }
        },
        {
          "name": "admin",
          "docs": [
            "The admin signer authorizing this whitelist update."
          ],
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "docs": [
            "Instructions sysvar for CPI validation."
          ],
          "address": "Sysvar1nstructions1111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "whitelistedPrograms",
          "type": {
            "vec": "pubkey"
          }
        }
      ]
    },
    {
      "name": "upgradeProgram",
      "discriminator": [
        223,
        236,
        39,
        89,
        111,
        204,
        114,
        37
      ],
      "accounts": [
        {
          "name": "accessManager",
          "docs": [
            "The access manager PDA for admin authorization."
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
            ]
          }
        },
        {
          "name": "program",
          "docs": [
            "The target program to upgrade.",
            "Writable because BPF Loader Upgradeable requires it during upgrade."
          ],
          "writable": true
        },
        {
          "name": "programData",
          "docs": [
            "The target program's data account (BPF Loader Upgradeable PDA)."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "program"
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
          "name": "buffer",
          "docs": [
            "Buffer account containing the new program bytecode."
          ],
          "writable": true
        },
        {
          "name": "upgradeAuthority",
          "docs": [
            "`AccessManager`'s PDA that acts as the upgrade authority for the target program."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  117,
                  112,
                  103,
                  114,
                  97,
                  100,
                  101,
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
              },
              {
                "kind": "arg",
                "path": "targetProgram"
              }
            ]
          }
        },
        {
          "name": "spill",
          "docs": [
            "Account to receive refunded rent from the old buffer."
          ],
          "writable": true
        },
        {
          "name": "authority",
          "docs": [
            "The admin signer authorizing the upgrade."
          ],
          "signer": true
        },
        {
          "name": "instructionsSysvar",
          "address": "Sysvar1nstructions1111111111111111111111111"
        },
        {
          "name": "bpfLoaderUpgradeable",
          "address": "BPFLoaderUpgradeab1e11111111111111111111111"
        },
        {
          "name": "rent",
          "docs": [
            "Rent sysvar required by BPF Loader Upgradeable."
          ],
          "address": "SysvarRent111111111111111111111111111111111"
        },
        {
          "name": "clock",
          "docs": [
            "Required by BPF Loader Upgradeable's upgrade instruction"
          ],
          "address": "SysvarC1ock11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "targetProgram",
          "type": "pubkey"
        }
      ]
    }
  ],
  "accounts": [
    {
      "name": "accessManager",
      "discriminator": [
        129,
        207,
        58,
        151,
        47,
        246,
        92,
        35
      ]
    }
  ],
  "events": [
    {
      "name": "accessManagerTransferAccepted",
      "discriminator": [
        244,
        139,
        47,
        82,
        213,
        147,
        38,
        88
      ]
    },
    {
      "name": "accessManagerTransferCancelled",
      "discriminator": [
        200,
        239,
        227,
        176,
        191,
        21,
        219,
        62
      ]
    },
    {
      "name": "accessManagerTransferProposed",
      "discriminator": [
        114,
        132,
        13,
        147,
        11,
        198,
        82,
        88
      ]
    },
    {
      "name": "programUpgradedEvent",
      "discriminator": [
        187,
        15,
        83,
        36,
        251,
        96,
        19,
        210
      ]
    },
    {
      "name": "roleGrantedEvent",
      "discriminator": [
        221,
        21,
        92,
        108,
        136,
        12,
        105,
        112
      ]
    },
    {
      "name": "roleRevokedEvent",
      "discriminator": [
        104,
        105,
        52,
        114,
        39,
        94,
        217,
        251
      ]
    },
    {
      "name": "upgradeAuthorityClaimedEvent",
      "discriminator": [
        89,
        129,
        108,
        54,
        149,
        105,
        39,
        221
      ]
    },
    {
      "name": "upgradeAuthorityTransferCancelledEvent",
      "discriminator": [
        35,
        172,
        94,
        180,
        37,
        229,
        123,
        63
      ]
    },
    {
      "name": "upgradeAuthorityTransferProposedEvent",
      "discriminator": [
        63,
        200,
        21,
        91,
        27,
        82,
        52,
        61
      ]
    },
    {
      "name": "upgradeAuthorityTransferredEvent",
      "discriminator": [
        79,
        145,
        216,
        157,
        39,
        178,
        134,
        42
      ]
    },
    {
      "name": "whitelistedProgramsUpdatedEvent",
      "discriminator": [
        174,
        165,
        182,
        103,
        180,
        63,
        131,
        198
      ]
    }
  ],
  "errors": [
    {
      "code": 6000,
      "name": "unauthorized",
      "msg": "Unauthorized: caller does not have required role"
    },
    {
      "code": 6001,
      "name": "invalidRoleId",
      "msg": "Invalid role ID"
    },
    {
      "code": 6002,
      "name": "cannotRemoveLastAdmin",
      "msg": "Cannot remove the last admin"
    },
    {
      "code": 6003,
      "name": "roleNotGranted",
      "msg": "Account does not have the specified role"
    },
    {
      "code": 6004,
      "name": "invalidSysenv",
      "msg": "Invalid sysenv: cross-program invocation from unexpected program"
    },
    {
      "code": 6005,
      "name": "signerRequired",
      "msg": "Account must be a signer"
    },
    {
      "code": 6006,
      "name": "cpiNotAllowed",
      "msg": "CPI calls not allowed"
    },
    {
      "code": 6007,
      "name": "programMismatch",
      "msg": "Program account does not match target_program"
    },
    {
      "code": 6008,
      "name": "zeroAccount",
      "msg": "Zero account is not allowed"
    },
    {
      "code": 6009,
      "name": "duplicateWhitelistedProgram",
      "msg": "Duplicate entry in whitelisted programs list"
    },
    {
      "code": 6010,
      "name": "unauthorizedDeployer",
      "msg": "Only the program's upgrade authority can call initialize"
    },
    {
      "code": 6011,
      "name": "authorityMismatch",
      "msg": "New authority account does not match instruction parameter"
    },
    {
      "code": 6012,
      "name": "pendingTransferAlreadyExists",
      "msg": "A pending transfer for this target program already exists"
    },
    {
      "code": 6013,
      "name": "noPendingTransfer",
      "msg": "No pending transfer for this target program"
    },
    {
      "code": 6014,
      "name": "selfTransfer",
      "msg": "Cannot transfer upgrade authority to the current authority PDA"
    },
    {
      "code": 6015,
      "name": "tooManyPendingTransfers",
      "msg": "Maximum number of concurrent pending transfers reached"
    },
    {
      "code": 6016,
      "name": "noPendingAccessManagerTransfer",
      "msg": "No pending access manager transfer to accept or cancel"
    },
    {
      "code": 6017,
      "name": "pendingAccessManagerTransferAlreadyExists",
      "msg": "A pending access manager transfer already exists"
    },
    {
      "code": 6018,
      "name": "invalidProposedAccessManager",
      "msg": "Proposed access manager address is invalid"
    },
    {
      "code": 6019,
      "name": "accessManagerSelfTransfer",
      "msg": "Cannot transfer access manager to the current access manager"
    }
  ],
  "types": [
    {
      "name": "accessManager",
      "docs": [
        "Central role-based access control registry shared across all Solana IBC programs.",
        "",
        "Every program that requires permissioned operations (e.g. relaying, pausing,",
        "admin configuration) delegates authorization checks to this account.",
        "It stores a list of roles with their members and a whitelist of program IDs",
        "that are allowed to invoke admin-gated instructions via CPI.",
        "",
        "Role IDs are opaque `u64` values — the access manager does not interpret them.",
        "Consuming programs define and enforce what each role ID means."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "roles",
            "type": {
              "vec": {
                "defined": {
                  "name": "roleData"
                }
              }
            }
          },
          {
            "name": "whitelistedPrograms",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "pendingAuthorityTransfers",
            "docs": [
              "Pending upgrade authority transfers, one per managed program.",
              "",
              "Uses a bounded `Vec` rather than a single `Option` to support concurrent",
              "transfers — with timelocked multisigs, a single `Option` would require",
              "N sequential propose/accept cycles (N × timelock waits). With a `Vec`,",
              "all proposes can be batched in one multisig vote.",
              "",
              "A single shared upgrade authority PDA (without `target_program` in the",
              "seed) was considered but rejected: `SetAuthority` must be called once",
              "per program regardless, so a shared PDA saves no transactions, while",
              "per-program PDAs limit the blast radius of bugs and let Anchor's seeds",
              "constraint tie each signer to exactly one target program."
            ],
            "type": {
              "vec": {
                "defined": {
                  "name": "pendingAuthorityTransfer"
                }
              }
            }
          }
        ]
      }
    },
    {
      "name": "accessManagerTransferAccepted",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "oldAccessManager",
            "type": "pubkey"
          },
          {
            "name": "newAccessManager",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "accessManagerTransferCancelled",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "accessManager",
            "type": "pubkey"
          },
          {
            "name": "cancelledAccessManager",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "accessManagerTransferProposed",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "currentAccessManager",
            "type": "pubkey"
          },
          {
            "name": "proposedAccessManager",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "pendingAuthorityTransfer",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "targetProgram",
            "type": "pubkey"
          },
          {
            "name": "newAuthority",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "programUpgradedEvent",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "program",
            "type": "pubkey"
          },
          {
            "name": "authority",
            "type": "pubkey"
          },
          {
            "name": "timestamp",
            "type": "i64"
          }
        ]
      }
    },
    {
      "name": "roleData",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "roleId",
            "type": "u64"
          },
          {
            "name": "members",
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "roleGrantedEvent",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "roleId",
            "type": "u64"
          },
          {
            "name": "account",
            "type": "pubkey"
          },
          {
            "name": "grantedBy",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "roleRevokedEvent",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "roleId",
            "type": "u64"
          },
          {
            "name": "account",
            "type": "pubkey"
          },
          {
            "name": "revokedBy",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "upgradeAuthorityClaimedEvent",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "program",
            "type": "pubkey"
          },
          {
            "name": "sourceAccessManager",
            "type": "pubkey"
          },
          {
            "name": "newAuthority",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "upgradeAuthorityTransferCancelledEvent",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "program",
            "type": "pubkey"
          },
          {
            "name": "cancelledAuthority",
            "type": "pubkey"
          },
          {
            "name": "cancelledBy",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "upgradeAuthorityTransferProposedEvent",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "program",
            "type": "pubkey"
          },
          {
            "name": "currentAuthority",
            "type": "pubkey"
          },
          {
            "name": "proposedAuthority",
            "type": "pubkey"
          },
          {
            "name": "proposedBy",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "upgradeAuthorityTransferredEvent",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "program",
            "type": "pubkey"
          },
          {
            "name": "oldAuthority",
            "type": "pubkey"
          },
          {
            "name": "newAuthority",
            "type": "pubkey"
          },
          {
            "name": "acceptedBy",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "whitelistedProgramsUpdatedEvent",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "oldPrograms",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "newPrograms",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "updatedBy",
            "type": "pubkey"
          }
        ]
      }
    }
  ]
};
