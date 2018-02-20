
getpeers() {

  p2p_port=46656
  p2p_addr_env=PHOTON_NODE_PORT_${p2p_port}_TCP_ADDR

  env                    | \
    grep "$p2p_addr_env" | \
    sed -e "s/.*=\(.*\)/\1:$p2p_port/" | \
    paste -sd ',' -
}

export PHOTOND_P2P_SEEDS=$(getpeers)

echo "found P2P peers: $PHOTOND_P2P_SEEDS"

/photond start
