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
|   market   |       5 |
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

##### v0.34.0

1. Extend authz implementation for DeploymentDeposit to allow grantee re-use of unspent funds.
    - Example of previous behavior:
      Deployment authz granted from account B (grantor) to account A (grantee) in amount of 5AKT.
      Deployment is created with authorized spend and deposit amount of 3AKT.
      Deployment spends 1.5AKT and lease was closed. 1.5AKT remainder is returned to the grantor, and authorization has 2AKT left to spend
    - Example of new behavior:
      Deployment authz granted from account B (grantor) to account A (grantee) in amount of 5AKT.
      Deployment is created with authorized spend and deposit amount of 3AKT.Deployment spends 1.5AKT and lease was closed.
      1.5AKT remainder is returned to the grantor, and authorization is updated and has 3.5AKT left to spend.
2. Don’t allow multiple grants from different grantors to be used for deposit on same deployment.
   This issue may lead to a case where all remaining funds after deployment is closed are returned to last grantor.
   Such use case has been guarded against and only one authz depositor will be allowed per deployment

##### v0.32.0

1. remove checking if provider has active leases during provider update transactions. This check was iterating thru all existing leases on the network causing gas and thus transaction fees go to up to 3AKT which is way above desired values. Initial intention of check was to prevent provider changing attributes that is in use by active leases. Akash Network team will reintroduce check by adding secondary indexes in future network upgrades.
2. remove secondary index for market store which was never user.

- Migrations
    - market `4 -> 5`

##### v0.30.0

1. fix `MatchGSpec` which used during Bid validation. Previous upgrade **v0.28.0** brought up resources offer.Existing implementation of `MatchGSpec` improperly validates offer against group spec, which rejects bids on multi-service deployments with unequal amount of replicas.

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
