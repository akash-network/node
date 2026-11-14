FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/akt-reporter /usr/bin/akt-reporter
ENV AKT_REPORTER_HOME=/home/akt-reporter
USER 10001:10001
WORKDIR /home/akt-reporter
ENTRYPOINT ["/usr/bin/akt-reporter"]
CMD ["start"]
