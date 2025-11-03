FROM ubuntu:noble
LABEL "org.opencontainers.image.source"="https://github.com/akash-network/node"

RUN \
    apt update \
 && apt install -y \
        curl \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/\* /tmp/\* /var/tmp/*

COPY ./akash /bin/

EXPOSE 26656 26657 26658
