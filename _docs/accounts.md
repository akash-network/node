# Accounts Design

* [Account data model](#account-data-model)
* [Common validations](#common-validations)
* [Commands](#commands)
  * [Create User Account](#create-user-account)
  * [Update Datacenter Accounts](#update-datacenter-accounts)
  * [Transfer Tokens](#transfer-tokens)
  * [Get Account Details](#get-account-details)
* [Open Questions](#open-questions)


## Account data model:

```proto3
message pubkey {
  enum type {
    USER;
    DATACENTER;
  }
  uint64 balance;
  message Provider {
    // datacenter specific variables
    string address;
  }
}
```

## Common validations for each transaction
* pubkey is valid - character length, etc.
* tx is signiture is valid - cryptographic check

## Commands:

### Create User Account

command:
```sh
photon tx create -type [user|datacenter] -name <accountname>
```
transaction:
```proto3
{
  string pubkey;
  enum type {
    USER;
    DATACENTER;
  }
}
```
validations:
* type is "user" or "datacenter"
state change:
* pubkey entery in DB has type attribute set


### Update Datacenter Accounts

command:
```sh
photon tx update -r <resources> -name <accountname>
```
transaction:
```proto3
{
  string pubkey;
  message Provider {
    // datacenter specific variables
    string address;
  }
}
```
validations:
* account with pubkey is type datacenter
* address conforms to IPv4/6 specification or DNS name
* resources conforms to schema TBD

state change:
* pubkey entry in DB sets adderess and/or resource values


### Transfer Tokens

command:
```sh
photon transfer --name=<accountname> --amount=<amount> --to=<address> --sequence=<integer>
```
transaction:
```proto3
{
  string pubkey;
  uint64 amount;
  string destination;
  uint64 sequence;
}
```

validations:

* pubkey account has balance >= amount
* squence is > last sequence
* destination exists

state change:
* pubkey account balance is decremented amount
* destination account is incremented amount


### Get Account Details

command:
```sh
photon account query --address=<publickey>
```
request data:
```proto3
{
  string pubkey;
}
```

validations:
* none

state change:
* none

## Open Questions:
* use same model for user and datacenter and leave some fields empty for users?
* "send" tokens or "transfer" tokens
* transactions include fees / gas
