# Smart Contract Dev Setup

## Parity

## Account Setup

* Install Parity https://www.parity.io/
* Open parity. Should open a browser tab
* Click 'SETTINGS' -> 'Parity'
* Under 'chain/network to sync' select 'Parity syncs to the Kovan test network'
* Click 'ACCOUNTS'
* Click '+ ACCOUNT'
* Create the new account
* Add Kovan Eth to the account https://github.com/kovan-testnet/faucet

### Deploy and Watch Master Contract

In Parity on the Kovan testnet...
* Click 'CONTRACTS'
* Click 'Develop'
* Paste in Solidity code
* Click 'Compile'
* Under 'Select a contract' select ':Master'
* Click 'Deploy'
* Enter your account password to send the transaction
* Copy the 'ABI Definition'
* Click 'CONTRACTS'
* Click '+ WATCH'
* Make sure 'Custom Contract' is selected and click 'Next'
* Paste in the contract address and contract abi
* Click '+ADD CONTRACT'

### Use a Watched Contract

In Parity on the Kovan testnet...
* Click 'CONTRACTS'
* Click a contract
* Click '> EXECUTE'

## Remix

* Go to http://remix.ethereum.org/
* Paste in Solidity code
* Do Start Compile
* Go to the Run tab
* Select Environment -> JavaScript VM
* Select the Maser contract
* Click Create
