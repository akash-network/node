Reference for usage:
http://cosmos-sdk.readthedocs.io/en/latest/basecoin-basics.html
http://cosmos-sdk.readthedocs.io/en/latest/basecoin-tool.html

Setup:
open a terminal for the client
```sh
make build
./client keys new cool
./client keys new friend
```

copy the address output after creating the keys for cool
open new terminal for the node

```sh
./node init <the address you copied>
./node start
```

wait a second, if blocks are not streaming in the terminal something is wrong

go back to the client terminal

```sh
./client init --node=tcp://localhost:46657 --genesis=$HOME/.demonode/genesis.json
```

Notes: the --genesis file is created duing node init and only exists on the node machine. If the client is not on the node machine the node will have to send the client the genesis file.

in the client termianl

```sh
ME=$(./client keys get cool | awk '{print $2}')
YOU=$(./client keys get friend | awk '{print $2}')
./client query account $ME
```

this should output the balance of the ME account

to send a transaciton from the client terminal
```sh
./client tx send --name=cool --amount=1000mycoin --to=$YOU --sequence=1
./client query account $YOU
```

this should output the balance of the YOU account
