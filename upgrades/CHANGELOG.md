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
|   market   |       3 |
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

##### v0.24.0

1. Enforce **Minimum Validators commission** using onchain parameter. Default value is set to 5%.
   During upgrade each validator with commission less than 5% will be updated to 5%.
2. Introduce **Minimum Initial Deposit** for governance proposal using onchain parameter. 
   Proposal originator must deposit at least **Minimum Initial Deposit** for proposal transaction to succeed. Default value is set to 40% of MinDeposit.
3. Fix dangling Escrow Payments. Some escrow payments remain in open state when actual escrow account is closed.
4. Deployment store is updated with v1beta3/ResourceUnits (added GPU unit) 
5. Market store is updated with v1beta3/ResourceUnits (added GPU unit)
6. Introduce **Take Pay**

- Stores
  - Added
    - `agov`
    - `astaking`
    - `feegrant`
    - `take`

- Migrations
  - deployment 2 -> 3
  - market 2 -> 3
  
##### v0.20.0

##### v0.28.0

##### v0.15.0 (upgrade name `akash_v0.15.0_cosmos_v0.44.x`)
