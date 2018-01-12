## Accounts Design


#### Questions:
* use same model for user and datacenter and leave some fields empty for users?
* "send" tokens or "transfer" tokens
* transactions include fees / gas

#### Account data model:

```go
{
  name: "string",
  type: ["user", "datacenter"],
  pubkey: "string",
  balance: "uint64"
}
```


#### Datacenter account data model:

```go
{
  address: "IP address",
  resources:
    {
      "some measure of total resources"
    }
}
```

#### Common validations: happen for each tx
* pubkey is valid - character length, etc.
* tx is signiture is valid - cryptographic check

#### Commands:
1. **create user account**


    command:
    ```sh
    photon account new -n <accountname> -t user
    ```
    transaction:
    ```go
    {
      pubkey: "string",
      type: ["user" | "datacenter"]
    }
    ```
    validations:
    * type is "user" or "datacenter"
    state change:
    * pubkey entery in DB has type attribute set


2. **update datacenter accounts**

    command:
    ```sh
    photon account update -a <address> -r <resources>
    ```
    transaction:
    ```go
    {
      pubkey: "string",
      address: "IP address"
      resources: {
        "some measure of total resources"
      }
    }
    ```
    validations:
    * account with pubkey is type datacenter
    * address conforms to IPv4/6 specification
    * resources conforms to schema TBD

    state change:
    * pubkey entry in DB sets adderess and/or resource values


3. **transfer tokens**

    command:
    ```sh
    photon transfer --name=<accountname> --amount=<amount> --to=<address> --sequence=<integer>
    ```
    transaction:
    ```go
    {
      pubkey: "string",
      amount: "uint64",
      destination: "string",
      sequence: "uint64",
      fee: "uint64"
    }
    ```

    validations:

    * pubkey account has balance >= amount + fee
    * squence is > last sequence

    state change:

    * pubkey account balance is decremented amount + fee
    * destination account is incremented fee
    * block creater address is credited fee


4. **get account details**

    command:
    ```sh
    photon account query -n <accountname> [--key=<publickey>]
    ```
    transaction:
    ```go
    {
      pubkey: "string",
    }
    ```

    validations:
    * none

    state change:
    * none

