FROM ubuntu:noble

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates python3 libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

ARG ROLE
COPY package/ /opt/akt/
RUN test "$ROLE" = relayer -o "$ROLE" = reporter \
    && test -x "/opt/akt/bin/akt-$ROLE" \
    && ln -s "/opt/akt/bin/akt-$ROLE" /usr/local/bin/akt-operator \
    && mkdir /data && chown 10001:10001 /data

USER 10001:10001
WORKDIR /data
ENTRYPOINT ["akt-operator"]
CMD ["--help"]
