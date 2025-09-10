# LocalOsmosis

LocalOsmosis is a complete Osmosis testnet containerized with Docker and orchestrated with a simple docker-compose file. LocalOsmosis comes preconfigured with opinionated, sensible defaults for a standard testing environment.

LocalOsmosis comes in two flavors:

1. No initial state: brand new testnet with no initial state.
2. With mainnet state: creates a testnet from a mainnet state export

Both ways, the chain-id for LocalOsmosis is set to 'localosmosis'.

## Prerequisites

Ensure you have docker and docker-compose installed:

```sh
# Docker
sudo apt-get remove docker docker-engine docker.io
sudo apt-get update
sudo apt install docker.io -y

# Docker compose
sudo apt install docker-compose -y
```

## 1. LocalOsmosis - No Initial State

The following commands must be executed from the root folder of the Osmosis repository.

1. Make any change to the osmosis code that you want to test

2. Initialize LocalOsmosis:

```bash
make localnet-init
```

The command:

- Builds a local docker image with the latest changes
- Cleans the `$HOME/.osmosisd-local` folder

3. Start LocalOsmosis:

```bash
make localnet-start
```

> Note
>
> You can also start LocalOsmosis in detach mode with:
>
> `make localnet-startd`

4. (optional) Add your validator wallet and 9 other preloaded wallets automatically:

```bash
make localnet-keys
```

- These keys are added to your `--keyring-backend test`
- If the keys are already on your keyring, you will get an `"Error: aborted"`
- Ensure you use the name of the account as listed in the table below, as well as ensure you append the `--keyring-backend test` to your txs
- Example: `osmosisd tx bank send lo-test2 osmo1cyyzpxplxdzkeea7kwsydadg87357qnahakaks --keyring-backend test --chain-id localosmosis`

5. You can stop chain, keeping the state with

```bash
make localnet-stop
```

6. When you are done you can clean up the environment with:

```bash
make localnet-clean
```

## 2. LocalOsmosis - With Mainnet State

Running an osmosis network with mainnet state is now as easy as setting up a stateless localnet.

1. Set up a mainnet node, allowing it to start and sync a few blocks. Then, stop it at the desired height where you want to fork the network.

2. There are now two options you can choose from:

    - **Mainnet is on version X, and you want to create a testnet on version X.**

      On version X, run:

       ```bash
       osmosisd in-place-testnet localosmosis osmo12smx2wdlyttvyzvzg54y2vnqwq2qjateuf7thj
       ```

      Where the first input is the desired chain-id of the new network and the second input is the desired validator operator address (where you vote from).
      The address provided above is included in the localosmosis keyring under the name 'val'.

      You now have a network you own with the mainnet state on version X.

    - **Mainnet is on version X, and you want to create a testnet on version X+1.**

      On version X, build binary and run:

       ```bash
       osmosisd in-place-testnet localosmosis osmo12smx2wdlyttvyzvzg54y2vnqwq2qjateuf7thj --trigger-testnet-upgrade=vXX
       ```

      where vXX indicates the next version that mainnet needs to be upgraded to. For example when current mainnet state is at v26, the flag value should be `--trigger-testnet-upgrade=v27`.

      The first input is the desired chain-id of the new network and the second input is the desired validator operator address (where you vote from).
      The address provided above is included in the localosmosis keyring under the name 'val'.

      The network will start and hit 10 blocks, at which point the upgrade will trigger and the network will halt.

      Then, on version X+1, run:

       ```bash
       osmosisd start
       ```

You now have a network you own with the mainnet state on version X+1.

### Removing unnecessary log lines
Running in-place-testnet would be resulting in noise in the log lines that looks something like this.
```
{"level": "info", "module": "server", "module": "consensus", "err": "cannot find validator 122 in valSet of size 1: invalid valid ator index", "time": "2024-10-21T21:34:30Z" ,"message": "failed attempting to add vote"} {"level": "error" , "module": "server", "module":" consensus" ,"height" :22731539, "round" :0, "peer": "27e14df66c9e4cd6b176b@dcabadfa 96750f911", "msg_type": "*consensus. VoteMessage", "err": "error adding vote", "time": "2024-10-21T21:34:307","message":"failed to process message"} {"level": "info" ,"module": "server"
', "module": "consensus", "err": "cannot find validator 107 in valSet of size 1: invalid valid
ator index", "time": "2024-10-21T21:34:30Z", "message": "failed attempting to add vote"}
```

To remove the noise, remove existing addrbook.json from config folder, and replace it with an empty json.

### Changing In Place Testnet Parameters

The settings for in place testnet are done in https://github.com/osmosis-labs/osmosis/blob/bb7a94e2561cc63b60ee76ec71a3e04e9688b22c/app/app.go#L773. Modify the parameters in `InitOsmosisAppForTestnet` to modify in place testnet parameters. For example, if you were to modify epoch hours, you would be modifying https://github.com/osmosis-labs/osmosis/blob/bb7a94e2561cc63b60ee76ec71a3e04e9688b22c/app/app.go#L942-L967 .


## LocalOsmosis Accounts and Keys

LocalOsmosis is pre-configured with one validator and 9 accounts with ION and OSMO balances.

| Account   | Address                                                                                                | Mnemonic                                                                                                                                                                   |
|-----------|--------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| lo-val    | `osmo1phaxpevm5wecex2jyaqty2a4v02qj7qmlmzk5a`<br/>`osmovaloper1phaxpevm5wecex2jyaqty2a4v02qj7qm9v24r6` | `satisfy adjust timber high purchase tuition stool faith fine install that you unaware feed domain license impose boss human eager hat rent enjoy dawn`                    |
| lo-test1  | `osmo1cyyzpxplxdzkeea7kwsydadg87357qnahakaks`                                                          | `notice oak worry limit wrap speak medal online prefer cluster roof addict wrist behave treat actual wasp year salad speed social layer crew genius`                       |
| lo-test2  | `osmo18s5lynnmx37hq4wlrw9gdn68sg2uxp5rgk26vv`                                                          | `quality vacuum heart guard buzz spike sight swarm shove special gym robust assume sudden deposit grid alcohol choice devote leader tilt noodle tide penalty`              |
| lo-test3  | `osmo1qwexv7c6sm95lwhzn9027vyu2ccneaqad4w8ka`                                                          | `symbol force gallery make bulk round subway violin worry mixture penalty kingdom boring survey tool fringe patrol sausage hard admit remember broken alien absorb`        |
| lo-test4  | `osmo14hcxlnwlqtq75ttaxf674vk6mafspg8xwgnn53`                                                          | `bounce success option birth apple portion aunt rural episode solution hockey pencil lend session cause hedgehog slender journey system canvas decorate razor catch empty` |
| lo-test5  | `osmo12rr534cer5c0vj53eq4y32lcwguyy7nndt0u2t`                                                          | `second render cat sing soup reward cluster island bench diet lumber grocery repeat balcony perfect diesel stumble piano distance caught occur example ozone loyal`        |
| lo-test6  | `osmo1nt33cjd5auzh36syym6azgc8tve0jlvklnq7jq`                                                          | `spatial forest elevator battle also spoon fun skirt flight initial nasty transfer glory palm drama gossip remove fan joke shove label dune debate quick`                  |
| lo-test7  | `osmo10qfrpash5g2vk3hppvu45x0g860czur8ff5yx0`                                                          | `noble width taxi input there patrol clown public spell aunt wish punch moment will misery eight excess arena pen turtle minimum grain vague inmate`                       |
| lo-test8  | `osmo1f4tvsdukfwh6s9swrc24gkuz23tp8pd3e9r5fa`                                                          | `cream sport mango believe inhale text fish rely elegant below earth april wall rug ritual blossom cherry detail length blind digital proof identify ride`                 |
| lo-test9  | `osmo1myv43sqgnj5sm4zl98ftl45af9cfzk7nhjxjqh`                                                          | `index light average senior silent limit usual local involve delay update rack cause inmate wall render magnet common feature laundry exact casual resource hundred`       |
| lo-test10 | `osmo14gs9zqh8m49yy9kscjqu9h72exyf295afg6kgk`                                                          | `prefer forget visit mistake mixture feel eyebrow autumn shop pair address airport diesel street pass vague innocent poem method awful require hurry unhappy shoulder`     |

To list all keys in the keyring named `test`
```bash
osmosisd keys list --keyring-backend test
```

To import an account into the keyring `test`. NOTE: replace the address with any of the above user accounts.
```bash
osmosisd keys add osmo1cyyzpxplxdzkeea7kwsydadg87357qnahakaks --keyring-backend test --recover
```

## Tests

### Software-upgrade test

To test a software upgrade, you can use the `submit_upgrade_proposal.sh` script located in the `scripts/` folder. This script automatically creates a proposal to upgrade the software to the specified version and votes "yes" on the proposal. Once the proposal passes and the upgrade height is reached, you can update your localosmosis instance to use the new version.

#### Usage

To use the script:

1. make sure you have a running LocalOsmosis instance

2. run the following command:

```bash
./scripts/submit_upgrade_proposal.sh <upgrade version>
```

Replace `<upgrade version>` with the version of the software you want to upgrade to, for example. If no version is specified, the script will default to `v15` version.

The script does the following:

- Creates an upgrade proposal with the specified version and description.
- Votes "yes" on the proposal.

#### Upgrade

Once the upgrade height is reached, you need to update your `localosmosis` instance to use the new software.

There are two ways to do this:

1. Change the image in the `docker-compose.yml` file to use the new version, and then restart LocalOsmosis using `make localnet-start`. For example:

```yaml
services:
  osmosisd:
    image: <NEW_IMAGE_I_WANT_TO_USE>
    # All this needs to be commented to don't build the image with local changes
    # 
    # build:
    #     context: ../../
    #     dockerfile: Dockerfile
    #     args:
    #     RUNNER_IMAGE: alpine:3.17
    #     GO_VERSION: 1.22
```

2. Checkout the Osmosis repository to a different `ref` that includes the new version, and then rebuild and restart LocalOsmosis using `make localnet-start`. Make sure to don't delete your `~/.osmosisd-local` folder.

### Create a pool
You can create a concentrated liquidity pool in `localosmosis`:
```bash
osmosisd tx concentratedliquidity create-pool uion uosmo 100 0.01 --from osmo1cyyzpxplxdzkeea7kwsydadg87357qnahakaks --chain-id localosmosis -b sync --keyring-backend test --fees 3000uosmo --gas 1000000
```
NOTE: Check `--from` and `--keyring-backend`. See also: [LocalOsmosis Accounts and Keys](#localosmosis-accounts-and-keys)

## FAQ

Q: How do I enable pprof server in localosmosis?

A: everything but the Dockerfile is already configured. Since we use a production Dockerfile in localosmosis, we don't want to expose the pprof server there by default. As a result, if you would like to use pprof, make sure to add `EXPOSE 6060` to the Dockerfile and rebuild the localosmosis image.
