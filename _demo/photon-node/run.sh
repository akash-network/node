
getpeers() {

  p2p_port=46656
  p2p_addr_env=AKASH_NODE_PORT_${p2p_port}_TCP_ADDR

  env                    | \
    grep "$p2p_addr_env" | \
    sed -e "s/.*=\(.*\)/\1:$p2p_port/" | \
    paste -sd ',' -
}

export AKASHD_P2P_SEEDS=$(getpeers)

echo "found P2P peers: $AKASHD_P2P_SEEDS"

/akashd start
