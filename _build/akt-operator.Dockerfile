FROM ubuntu:noble AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

ARG ROLE
COPY package/ /opt/akt/
RUN test "$ROLE" = relayer -o "$ROLE" = reporter \
    && test -x "/opt/akt/bin/akt-$ROLE" \
    && find /opt/akt -type d -exec chmod 0755 {} + \
    && mkdir /data && chown 10001:10001 /data

USER 10001:10001
WORKDIR /data
CMD ["--help"]

FROM runtime AS relayer
USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends python3-minimal \
    && rm -rf /var/lib/apt/lists/*
USER 10001:10001
ENTRYPOINT ["/opt/akt/bin/akt-relayer"]

FROM runtime AS reporter
ENTRYPOINT ["/opt/akt/bin/akt-reporter"]
