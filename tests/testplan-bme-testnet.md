# BME (Block Market Exchange) Testnet Testplan

## Overview

This testplan covers the BME module functionality for testnet validation. The BME module manages the conversion between AKT (Akash Token) and ACT (Akash Compute Token) using a vault system with collateral ratio-based circuit breaker mechanisms.

## Module Summary

- **Purpose**: Token burn/mint exchange mechanism for AKT ↔ ACT conversion
- **Key Features**:
  - AKT → ACT conversion (minting ACT)
  - ACT → AKT conversion (burning ACT)
  - Collateral Ratio (CR) based circuit breaker
  - Oracle price integration for swap calculations
  - Vault state tracking (balances, burned, minted)

## Prerequisites

### Testnet Environment Setup

- [ ] Testnet node running with BME module enabled
- [ ] Oracle module configured with AKT and ACT price feeds
- [ ] Test accounts with sufficient AKT balance
- [ ] Access to CLI (`akash`) or REST/gRPC endpoints
- [ ] Price feeder running and submitting prices

### Required Configuration

```yaml
# BME Module Parameters (verify defaults)
- circuit_breaker_warn_threshold: 11000  # 110% (basis points)
- circuit_breaker_halt_threshold: 10000  # 100% (basis points)
```

---

## Test Categories

### 1. Query Operations

#### TC-BME-Q01: Query BME Parameters

**Description**: Verify BME module parameters can be queried

**Steps**:
1. Query BME parameters via CLI:
   ```bash
   akash query bme params --output json
   ```
2. Query via REST:
   ```bash
   curl -s $NODE_API/akash/bme/v1/params
   ```

**Expected Results**:
- [ ] Response returns valid `Params` object
- [ ] `circuit_breaker_warn_threshold` is present and valid
- [ ] `circuit_breaker_halt_threshold` is present and valid

---

#### TC-BME-Q02: Query Vault State

**Description**: Verify vault state can be queried showing balances, burned, and minted amounts

**Steps**:
1. Query vault state via CLI:
   ```bash
   akash query bme vault-state --output json
   ```
2. Query via REST:
   ```bash
   curl -s $NODE_API/akash/bme/v1/vault-state
   ```

**Expected Results**:
- [ ] Response returns valid `VaultState` object
- [ ] `balances` array is present (may be empty initially)
- [ ] `burned` array is present (may be empty initially)
- [ ] `minted` array is present (may be empty initially)

---

#### TC-BME-Q03: Query Collateral Ratio

**Description**: Verify collateral ratio can be queried

**Steps**:
1. Query collateral ratio via CLI:
   ```bash
   akash query bme collateral-ratio --output json
   ```
2. Query via REST:
   ```bash
   curl -s $NODE_API/akash/bme/v1/collateral-ratio
   ```

**Expected Results**:
- [ ] Response returns valid `CollateralRatio` value
- [ ] Value is a decimal (e.g., "1.5" for 150%)
- [ ] Value is consistent with vault state

---

#### TC-BME-Q04: Query Circuit Breaker Status

**Description**: Verify circuit breaker status can be queried

**Steps**:
1. Query circuit breaker status via CLI:
   ```bash
   akash query bme circuit-breaker-status --output json
   ```
2. Query via REST:
   ```bash
   curl -s $NODE_API/akash/bme/v1/circuit-breaker-status
   ```

**Expected Results**:
- [ ] Response returns valid status: `Healthy`, `Warning`, or `Halt`
- [ ] `settlements_allowed` boolean is present
- [ ] `refunds_allowed` boolean is present
- [ ] In healthy state: both `settlements_allowed` and `refunds_allowed` should be `true`

---

### 2. Oracle Integration Tests

#### TC-BME-O01: Verify Oracle Price Availability

**Description**: Ensure oracle prices for AKT and ACT are available for BME operations

**Steps**:
1. Query AKT price:
   ```bash
   akash query oracle price uakt --output json
   ```
2. Query ACT price:
   ```bash
   akash query oracle price uact --output json
   ```

**Expected Results**:
- [ ] AKT price is available and non-zero
- [ ] ACT price is available and equals $1.00 (or configured value)
- [ ] Prices are recent (within configured staleness threshold)

---

#### TC-BME-O02: Price Impact on Swap Rate

**Description**: Verify swap rate calculation based on oracle prices

**Steps**:
1. Record current AKT price (e.g., $1.14)
2. Calculate expected swap rate: `AKT_price / ACT_price`
3. Perform a test conversion and verify actual rate

**Expected Results**:
- [ ] Swap rate = AKT_price / ACT_price
- [ ] Example: If AKT = $1.14 and ACT = $1.00, rate = 1.14
- [ ] Minting 100 AKT should produce ~114 ACT (minus any fees)

---

### 3. Burn/Mint Operations

#### TC-BME-BM01: AKT to ACT Conversion (Mint ACT)

**Description**: Test conversion of AKT to ACT through a deployment lease deposit

**Preconditions**:
- Test account has sufficient AKT balance
- Circuit breaker status is `Healthy`
- Oracle prices are available

**Steps**:
1. Record initial vault state
2. Record initial account balances
3. Create a deployment with AKT deposit:
   ```bash
   akash tx deployment create deployment.yaml --from $ACCOUNT --deposit 100000uakt
   ```
4. Query vault state after deposit
5. Verify ACT was minted

**Expected Results**:
- [ ] AKT transferred from account to BME vault
- [ ] ACT minted based on oracle price
- [ ] Vault state shows increased AKT balance
- [ ] Vault state shows increased minted ACT amount
- [ ] Escrow account funded with ACT

---

#### TC-BME-BM02: ACT to AKT Conversion (Settlement/Withdrawal)

**Description**: Test conversion of ACT to AKT during provider settlement

**Preconditions**:
- Active lease with ACT escrow balance
- Provider has pending earnings

**Steps**:
1. Record initial vault state
2. Record provider AKT balance
3. Trigger settlement (via lease close or payment withdrawal)
4. Query vault state after settlement
5. Verify provider received AKT

**Expected Results**:
- [ ] ACT burned from escrow
- [ ] AKT minted/released to provider
- [ ] Vault state shows increased burned ACT amount
- [ ] Provider received correct AKT amount based on oracle price

---

#### TC-BME-BM03: Refund Conversion (ACT to AKT)

**Description**: Test refund conversion when deployment closes

**Preconditions**:
- Active deployment with remaining ACT balance

**Steps**:
1. Record initial vault state
2. Record owner AKT balance
3. Close deployment:
   ```bash
   akash tx deployment close --dseq $DSEQ --from $ACCOUNT
   ```
4. Query vault state after close
5. Verify owner received AKT refund

**Expected Results**:
- [ ] Remaining ACT burned from escrow
- [ ] AKT sent to deployment owner
- [ ] Vault state updated correctly

---

### 4. Circuit Breaker Tests

#### TC-BME-CB01: Healthy State Operations

**Description**: Verify normal operations when circuit breaker is healthy

**Preconditions**:
- CR > warn_threshold (e.g., CR > 110%)

**Steps**:
1. Verify circuit breaker status is `Healthy`
2. Perform AKT → ACT conversion (deposit)
3. Perform ACT → AKT conversion (settlement)

**Expected Results**:
- [ ] All operations succeed
- [ ] `settlements_allowed = true`
- [ ] `refunds_allowed = true`

---

#### TC-BME-CB02: Warning State Monitoring

**Description**: Monitor system behavior when CR approaches warning threshold

**Note**: This test may require controlled testnet conditions

**Preconditions**:
- Ability to manipulate CR through deposits/withdrawals

**Steps**:
1. Monitor CR as it approaches warning threshold
2. Verify status changes to `Warning` when CR < warn_threshold

**Expected Results**:
- [ ] Status changes from `Healthy` to `Warning`
- [ ] Operations still allowed in warning state
- [ ] Warning events emitted (check logs)

---

#### TC-BME-CB03: Halt State Fallback

**Description**: Verify circuit breaker halt prevents ACT minting and falls back to AKT

**Note**: This test may require controlled testnet conditions or governance param changes

**Preconditions**:
- CR < halt_threshold (e.g., CR < 100%)

**Steps**:
1. Trigger circuit breaker halt condition
2. Attempt AKT → ACT deposit
3. Verify fallback to direct AKT settlement

**Expected Results**:
- [ ] Status is `Halt`
- [ ] New deposits use AKT directly (no ACT minting)
- [ ] Error `ErrCircuitBreakerActive` returned for ACT mint attempts
- [ ] Existing settlements and refunds may still be allowed

---

### 5. Ledger and Event Tests

#### TC-BME-L01: Transaction Ledger Recording

**Description**: Verify all burn/mint operations are recorded in the ledger

**Steps**:
1. Perform a burn/mint operation
2. Query events from the transaction
3. Verify `BMRecord` event is emitted

**Expected Results**:
- [ ] Event contains `burned_from` address
- [ ] Event contains `minted_to` address
- [ ] Event contains `burned` coin with price
- [ ] Event contains `minted` coin with price

---

#### TC-BME-L02: Block-level Ledger Sequencing

**Description**: Verify ledger sequence resets per block

**Steps**:
1. Perform multiple burn/mint operations in same block
2. Query ledger records
3. Verify sequence numbers

**Expected Results**:
- [ ] Each operation has unique sequence within block
- [ ] Sequence resets to 0 on new block (BeginBlocker)

---

### 6. Integration Tests

#### TC-BME-I01: Full Deployment Lifecycle

**Description**: Test complete deployment lifecycle with BME

**Steps**:
1. Create deployment with AKT deposit
2. Create provider bid (in ACT)
3. Accept bid, create lease
4. Run for several blocks
5. Provider withdraws earnings
6. Close deployment
7. Verify final balances

**Expected Results**:
- [ ] All conversions use correct oracle prices
- [ ] Provider receives correct AKT settlement
- [ ] Owner receives correct AKT refund
- [ ] Vault state reflects all operations

---

#### TC-BME-I02: Multiple Concurrent Deployments

**Description**: Test BME with multiple active deployments

**Steps**:
1. Create multiple deployments with different deposit amounts
2. Create leases for each
3. Trigger settlements at different times
4. Verify vault state consistency

**Expected Results**:
- [ ] All operations tracked correctly
- [ ] No race conditions in vault state
- [ ] Collateral ratio calculated correctly across all operations

---

### 7. Parameter Governance Tests

#### TC-BME-G01: Update BME Parameters via Governance

**Description**: Test updating BME parameters through governance proposal

**Steps**:
1. Submit governance proposal to update circuit breaker thresholds
2. Vote on proposal
3. Wait for proposal to pass
4. Verify new parameters applied

**Expected Results**:
- [ ] Proposal submitted successfully
- [ ] Parameters updated after proposal passes
- [ ] New thresholds take effect immediately

---

## Test Data Recording Template

For each test execution, record:

| Field | Value |
|-------|-------|
| Test ID | |
| Date | |
| Testnet | |
| Block Height | |
| Tester | |
| Result (Pass/Fail) | |
| Notes | |
| Transaction Hash(es) | |

---

## Metrics to Monitor

During testnet testing, monitor:

1. **Vault Metrics**:
   - Total AKT in vault
   - Total ACT minted
   - Total ACT burned
   - Collateral ratio over time

2. **Oracle Metrics**:
   - AKT price updates
   - Price staleness

3. **Circuit Breaker**:
   - Status changes
   - Time spent in each state

4. **Transaction Metrics**:
   - Burn/mint transaction count
   - Average conversion amounts
   - Failed transactions (circuit breaker halts)

---

## Known Limitations

1. **Controlled CR Testing**: Triggering circuit breaker halt may require significant testnet manipulation or governance parameter changes
2. **Oracle Dependency**: Tests depend on functioning oracle price feeds
3. **True Burn Implementation**: Uses true burn/mint instead of remint credits due to Cosmos SDK constraints

---

## Appendix: CLI Command Reference

### Query Commands

```bash
# Query BME parameters
akash query bme params

# Query vault state
akash query bme vault-state

# Query collateral ratio
akash query bme collateral-ratio

# Query circuit breaker status
akash query bme circuit-breaker-status
```

### REST Endpoints

```
GET /akash/bme/v1/params
GET /akash/bme/v1/vault-state
GET /akash/bme/v1/collateral-ratio
GET /akash/bme/v1/circuit-breaker-status
```

---

## References

- BME Module Source: `x/bme/`
- BME Keeper: `x/bme/keeper/keeper.go`
- E2E Tests: `tests/e2e/bme_cli_test.go`, `tests/e2e/bme_grpc_test.go`
- Documentation: `bme.md`
