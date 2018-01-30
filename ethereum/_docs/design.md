# Ethereum MVP

<img src="./dot/contracts.svg">

- [Applications](#applications)
  * [Order Matching Daemon](#order-matching-daemon)
    + [PSQL database](#psql-database)
    + [Go App](#go-app)
  * [Web Client](#web-client)
- [Contracts](#contracts)
  * [Master](#master)
    + [Functions](#functions)
      - [deployProvider](#deployprovider)
      - [deployClient](#deployclient)
      - [match](#match)
  * [Provider](#provider)
    + [State Varaibles:](#state-varaibles-)
      - [matchedClient](#matchedclient)
      - [networkAddress](#networkaddress)
    + [Functions](#functions-1)
      - [Provider](#provider-1)
      - [match](#match-1)
      - [cancel](#cancel)
      - [uncancel](#uncancel)
      - [withdrawal](#withdrawal)
  * [Client](#client)
    + [State Variables](#state-variables)
      - [matchedProvider](#matchedprovider)
      - [minimumBalance](#minimumbalance)
      - [matchStartTime](#matchstarttime)
      - [totalBilled](#totalbilled)
      - [unsettledBalance](#unsettledbalance)
      - [maxUnsettledBalance](#maxunsettledbalance)
    + [Functions](#functions-2)
      - [match](#match-2)
      - [setBill](#setbill)
      - [bill](#bill)
      - [cancel](#cancel-1)
      - [providerCancel](#providercancel)
      - [unCancel](#uncancel)
- [Modifiers](#modifiers)
  * [Parameterized](#parameterized)
  * [Maintainable](#maintainable)
  * [Delinquent](#delinquent)
  * [Cancellable](#cancellable)
  * [Matchable](#matchable)
  * [BadActor](#badactor)
  * [Ownable](#ownable)
  * [Payable](#payable)
- [Future Work](#future-work)
- [Open Questions](#open-questions)
  * [Fee Structure](#fee-structure)

## Applications

### Order Matching Daemon

#### PSQL database

* Tables mirror data within the contracts
* Tables: Clients, Providers

#### Go App

* Gets internal transactons from Master contract
* Creates records for addresses of all Deploy Client and Provider transactions
* Creates records for the state of all found Client and Provider transactions
* Iterates through unmatched Provder contracts searching for a matching Client contract
* Order matching precedence is closest match then time
* Calls the `match` function on the Master contract for the matched contracts
* Waits for a new Ethereum block and repeates this process

### Web Client

* For users and datacenters to Deploy Client or Provider contracts
* Integrates with Metamask for transaction signing
* Encrypts and seeds deployment manifest
* Looks up users in-progress contracts by ETH address
* Lists unmatched contracts
* For users to interact with their contract

## Contracts

### Master

We choose to deploy all Provider and Client contracts from a single Master contract in order to have the ability for programmatic contract discovery. If users deploy their own Provider or Client contracts we will not know where they are.

#### Functions

The master contract is deployed and Maintained by Overclock Labs
The master contract has two functions anyone can call and one function only the maintainer can call

##### deployProvider

* Deploys a Provider contract
* Callable by anyone

##### deployClient

* Deploys a Client contract
* Callable by anyone

##### match

* Matches a Provider contract with a Client contract. Matches must be done on a first come first serve basis. The only way to guarantee this is to restrict the parties which are allowed to match the Provider and Client contracts. The result of the matching is publicly viewable on the blockchain, therefore there is no risk that the maintainer can secretly give preference to certain orders without the potential of being exposed.

* Callable by the maintainer


### Provider

Extended by: Ownable, Parameterized, Matchable, Cancelable, Payable, BadActor

#### State Varaibles:

##### matchedClient

* The Ethereum address of the Client contract matched with this Provider contract

##### networkAddress

* The IP or URL of the provider which can be contacted to initiate manifest distribution

#### Functions

##### Provider

* Contract constructor
* Sets available resources, maintainer address, client constraints

##### match

* Tries to match with a Client contract
* Checks if Client contract has compatible parameters

##### cancel

* Marks the contract as canceled
* May charge the Provider an early cancellation fee

##### uncancel

* Marks the contract as not canceled
* Allows contract to be re-matched

##### withdrawal

* Sends the contract ETH balance to the maintainer

### Client

Extended by Ownable, Parameterized, Matchable, Cancelable, Payable, Deliquent

#### State Variables

##### matchedProvider

* The address of the matched Provider contract

##### minimumBalance

* The minimum ETH balance the client has promised to maintain in the contract

##### matchStartTime

* The time the contract was matched

##### totalBilled

* The total amount the Client has sent to the matched Provider

##### unsettledBalance

* The total amount the Client owes to the matched Provider

##### maxUnsettledBalance

* The amount that the client has promised not owe greater than

#### Functions

##### match

* Called by a Provider contract to attempt a match

##### setBill

* Determines the unsettled balance of the Client

##### bill

* Sends unsettled balance to the matched Provider

##### cancel

* Cancel the contract

##### providerCancel

* Allows the matched Provider to cancel the contract and withdraw all funds if the Client is delinquent

##### unCancel

* Uncancel the contract to enable matching

## Modifiers

* These are contracts that act as small modules of behavior and state to be extended by large contracts

### Parameterized

* List of contract parameters common between Client and Provider
* Examples: cpu, ram, cancelFee.

### Maintainable

* Allows an address to be the maintainer and transfer maintainership to another address

### Delinquent

* A contract can be marked as Delinquent is there are issues with payment

### Cancellable

* A contract can be marked canceled

### Matchable

* A contract can be marked matched

### BadActor

* A Provider can be marked as a bad actor

### Ownable

* Mark the address which deployed the contract as the owner

### Payable

* Allows ETH to be sent to a contract

## Future Work

* Multiple Clients matched per single Provider contract which lists aggregate resources. Available resource calculations are managed by the contract
* Oracle permission to mark Providers as bad actors
* Use a math library for accurate calculation of billing
* Blacklist URLs of bad acting Providers

## Open Questions

### Fee Structure


Which, or both, should be implemented?


Client Burden: client maintains a minimum balance, if balance falls below minimum, provider can cancel without being charged a fee
    - a provider can ensure he is always paid
    - a client has to 'waste' money by keeping it sitting in a contract


Provider Burden: if client misses too many payments, provider can cancel without incurring fees
    - a provider may not be paid for services
    - a client is allowed flexibility of payment
