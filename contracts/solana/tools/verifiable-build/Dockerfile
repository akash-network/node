# Pinned toolchain for verifiable SBF builds of akt_solana, ics07_tendermint and
# access_manager. Use scripts/verifiable-build.sh; never push this image.
# Agave publishes x86_64 Linux binaries only, so the image is linux/amd64.
# rust:1.89.0-slim-trixie: Anchor 0.32.1's release binary needs glibc 2.39.
FROM docker.io/library/rust:1.89.0-slim-trixie@sha256:9553b8397715ff026f4742266280d2b869de33493dc3e16d92dd8ec3534f6fb9

# Debian packages come from a fixed snapshot. protoc parses upstream .proto
# files for prost; make and perl build the vendored OpenSSL in Anchor's host
# IDL builds; the others fetch, unpack and patch sources.
RUN rm -f /etc/apt/sources.list.d/debian.sources \
 && echo 'deb [check-valid-until=no] http://snapshot.debian.org/archive/debian/20251001T000000Z trixie main' \
      > /etc/apt/sources.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends bzip2 curl libudev1 make patch perl protobuf-compiler \
 && rm -rf /var/lib/apt/lists/*

ARG AGAVE_VERSION=2.3.13
ARG AGAVE_SHA256=c43539ebdf6942472e8b87635d6ea55f428a51e3d0219f7b6f720fc6b19fade0
ARG PLATFORM_TOOLS_VERSION=v1.48
ARG PLATFORM_TOOLS_SHA256=a9d3157f937d5f6faf4321a35a8035e0f827114a663b0150db492e893ec27597
ARG ANCHOR_VERSION=0.32.1
ARG ANCHOR_SHA256=5f25b850ce80278507a98947833fcd48423391f6d145046ffb0c5fd130dec436

WORKDIR /tmp/tools
RUN curl -fsSL --retry 3 -o agave.tar.bz2 \
      "https://github.com/anza-xyz/agave/releases/download/v${AGAVE_VERSION}/solana-release-x86_64-unknown-linux-gnu.tar.bz2" \
 && echo "${AGAVE_SHA256}  agave.tar.bz2" | sha256sum -c - \
 && mkdir -p /opt/agave \
 && tar -xjf agave.tar.bz2 -C /opt/agave --strip-components=1 \
 && rm agave.tar.bz2
# cargo-build-sbf uses this cache directory instead of downloading the tools.
RUN curl -fsSL --retry 3 -o platform-tools.tar.bz2 \
      "https://github.com/anza-xyz/platform-tools/releases/download/${PLATFORM_TOOLS_VERSION}/platform-tools-linux-x86_64.tar.bz2" \
 && echo "${PLATFORM_TOOLS_SHA256}  platform-tools.tar.bz2" | sha256sum -c - \
 && mkdir -p "/root/.cache/solana/${PLATFORM_TOOLS_VERSION}/platform-tools" \
 && tar -xjf platform-tools.tar.bz2 -C "/root/.cache/solana/${PLATFORM_TOOLS_VERSION}/platform-tools" \
 && rm platform-tools.tar.bz2
RUN curl -fsSL --retry 3 -o anchor \
      "https://github.com/otter-sec/anchor/releases/download/v${ANCHOR_VERSION}/anchor-${ANCHOR_VERSION}-x86_64-unknown-linux-gnu" \
 && echo "${ANCHOR_SHA256}  anchor" | sha256sum -c - \
 && install -m 755 anchor /usr/local/bin/anchor \
 && rm anchor
# The repositories' rust-toolchain.toml files ask for these components.
RUN rustup component add --toolchain 1.89.0 clippy rustfmt

ENV PATH=/opt/agave/bin:$PATH
WORKDIR /
RUN solana --version && cargo-build-sbf --version && anchor --version && rustc --version
