# Upgrade handlers

## Upgrades history

### v0.22.0
 - enable min initial deposit check for all new proposals
 - upgrade validators with commission < %5
 
### v0.20.0
 - disable and remove Cosmos Inter Chain Accounts (ICA) due to issues discovered post v0.18.0
 
### v0.18.0
Key features that this upgrade enables are:
 - support for IP Leases on the Akash marketplace
 - initial phases of splitting provider services into microservices
 - code changes to support Cosmos Inter Chain Accounts (ICA) and IBC3.


### akash_v0.15.0_cosmos_v0.44.x

## How to add a new upgrade handler to the app

1. use [akash_v0.15.0_cosmos_v0.44.x](./akash_v0.15.0_cosmos_v0.44.x) as an example
2. import new upgrade module into [app/upgrades](../upgrades.go)
   ```go
   import (
       _ "github.com/akash-network/node/app/upgrades/akash_v0.15.0_cosmos_v0.44.x"
   )
3. Once imported, the upgrade will register itself, and `App` will initialize it during startup
4. To deregister obsolete upgrade simply remove respective import from [app/upgrades](../upgrades.go)
