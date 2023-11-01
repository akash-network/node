# Upgrades history

## Software upgrade

### Current module consensus versions

|   Module   | Version |
|:----------:|--------:|
|   audit    |       2 |
|    cert    |       2 |
| deployment |       3 |
|   escrow   |       2 |
|    agov    |       1 |
| inflation  |       1 |
|   market   |       4 |
|  provider  |       2 |
|  astaking  |       1 |
|    take    |       1 |

#### Upgrades

Template
-----

##### <Upgrade name>

###### Description

Goal of the upgrade here

- Stores (omit if all items below are not present in the upgrade)
    - added stores (omit if empty)
        - `store`
    - renamed stores (omit if empty)
        - `store`
    - deleted stores (omit if empty)
        - `store`

- Migrations (omit if all times below are not present in the upgrade)
    - deployment 2 -> 3
    - market 2 -> 3

Add new upgrades after this line based on the template above
-----

##### v0.28.0

1. Add resource offer for the bid, allowing providers to show details on the resources they offer, when order has wildcard resources, for example GPU.

- Migrations
    - market `3 -> 4`

##### v0.26.0

1. Enforce **Minimum Validators commission** using onchain parameter. Default value is set to 5%. This is carry-over from v0.24.0 upgrade, as this change was dry-run
 
##### v0.24.0

1. Update following stores to the `v1beta3`:
    - `audit`
    - `cert`
    - `deployment`
    - `market`
    - `provider`
    - `escrow`
2. Enforce **Minimum Validators commission** using onchain parameter. Default value is set to 5%.
   During upgrade each validator with commission less than 5% will be updated to 5%.
3. Introduce **Minimum Initial Deposit** for governance proposal using onchain parameter.
   Proposal originator must deposit at least **Minimum Initial Deposit** for proposal transaction to succeed. Default value is set to 40% of MinDeposit.
4. Fix dangling Escrow Payments. Some escrow payments remain in open state when actual escrow account is closed.
5. Deployment store is updated with `v1beta3/ResourceUnits` (added GPU unit)
   Migrate `MinDeposit` param to `MinDeposits`, allowing deployments to be paid in non-akt currencies.
6. Market store is updated with `v1beta3/ResourceUnits` (added GPU unit)
7. Introduce **Take Pay**

- Stores
    - Added
        - `agov`
        - `astaking`
        - `feegrant`
        - `take`

- Migrations
    - deployment `2 -> 3`
    - market `2 -> 3`

##### v0.20.0

1. Remove support of interchain accounts (aka ICA)

- Stores
    - Deleted
        - `icacontroller`
        - `icahost`

##### v0.18.0

1. Introduce interchain accounts (aka ICA)

- Stores
    - Added
        - `icacontroller`
        - `icahost`

##### v0.15.0 (upgrade name `akash_v0.15.0_cosmos_v0.44.x`)

1. Introduce Akash marketplace
2. Migrate store prefixes from v0.38/v0.39 to 0.40

- Stores
    - Added
      - `audit`
      - `cert`
      - `deployment`
      - `escrow`
      - `market`
      - `provider` 
