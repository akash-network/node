ARG SINK_TOOLCHAIN_IMAGE
FROM node:24.14.1-trixie-slim AS node
FROM ${SINK_TOOLCHAIN_IMAGE}

COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
 && apt-get update \
 && apt-get install -y --no-install-recommends lsof \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace/contracts/solana
