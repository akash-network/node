FROM debian:bullseye
LABEL "org.opencontainers.image.source"="https://github.com/akash-network/node"

ARG WASMVM_VERSION

COPY ./akash /bin/

EXPOSE 26656 26657 26658
