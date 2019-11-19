#!/bin/sh

source env.sh

saypass() {
  echo 11111111
}

do_init(){

  akashd init node-0 --chain-id akash-testnet

  (saypass; saypass) | akash keys add main
  (saypass; saypass) | akash keys add provider

  akashd add-genesis-account $(akash keys show main -a) 100000akash,100000000stake
  akashd add-genesis-account $(akash keys show provider -a) 100000akash,100000000stake

  akash config chain-id   akash-testnet
  akash config output     json
  akash config indent     true
  akash config trust-node true

  saypass | akashd gentx --name main --home-client "$AKASH_HOME"

  akashd collect-gentxs
  akashd validate-genesis
}

do_node() {
  akashd start
}

do_deploy() {
  saypass | akash tx deployment create deployment.yml --from main -y
}

do_mkprovider() {
  saypass | akash tx provider create provider.yaml --from provider -y
}

do_query() {
  akash query account $(akash keys show main -a)
  akash query account $(akash keys show provider -a)
  akash query deployment deployments
  akash query market orders
  akash query market bids
  akash query market leases
  akash query provider providers
}

do_deploy_close(){
  dseq="$1"
  saypass | akash tx deployment close -y \
    --owner "$(akash keys show main -a)" \
    --from main     \
    --dseq "$dseq"
}

do_bid() {
  dseq="$1"
  gseq="${2:-1}"
  oseq="${3:-1}"
  saypass | akash tx market bid-create -y \
    --owner "$(akash keys show main -a)" \
    --from provider \
    --dseq "$dseq"  \
    --gseq "$gseq"  \
    --oseq "$oseq"  \
    --price 1akash
}

do_bid_close() {
  dseq="$1"
  gseq="${2:-1}"
  oseq="${3:-1}"
  saypass | akash tx market bid-close -y \
    --owner "$(akash keys show main -a)" \
    --from provider \
    --dseq "$dseq"  \
    --gseq "$gseq"  \
    --oseq "$oseq"
}

do_order_close() {
  dseq="$1"
  gseq="${2:-1}"
  oseq="${3:-1}"
  saypass | akash tx market order-close -y \
    --owner "$(akash keys show main -a)" \
    --from main     \
    --dseq "$dseq"  \
    --gseq "$gseq"  \
    --oseq "$oseq"
}

case "$1" in
  init)
    do_init
    ;;
  node)
    do_node
    ;;
  deploy)
    do_deploy
    ;;
  query)
    do_query
    ;;
  mkprovider)
    do_mkprovider
    ;;
  bid)
    shift;
    do_bid "$@"
    ;;
  deploy-close)
    shift;
    do_deploy_close "$@"
    ;;
  bid-close)
    shift;
    do_bid_close "$@"
    ;;
  order-close)
    shift;
    do_order_close "$@"
    ;;
  *)
    echo "USAGE: $0 <init|node|query|deploy>" >&2
    exit 1
    ;;
esac
