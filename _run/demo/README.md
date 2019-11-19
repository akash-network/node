```sh
make clean
make
./run.sh init
./run.sh node
./run.sh query
./run.sh mkprovider
./run.sh deploy
./run.sh query

./run.sh bid 7
./run.sh query

./run.sh bid-close 7
./run.sh order-close 7

source env.sh
```
