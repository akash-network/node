# Simple usage with a mounted data directory:
# > docker build -t akash-build .
# > docker run -it -p 46657:46657 -p 46656:46656 -v ~/.akash:/akash/.akash akash-build akash init
# > docker run -it -p 46657:46657 -p 46656:46656 -v ~/.akash:/akash/.akash akash-build akash start
FROM golang:alpine AS build-env

# Set up dependencies
ENV PACKAGES curl make git libc-dev bash gcc linux-headers eudev-dev python3

# Set working directory for the build
WORKDIR /go/src/github.com/ovrclk/akash

# Add source files
COPY . .

# Install minimum necessary dependencies, build akash, remove packages
RUN apk add --no-cache $PACKAGES && \
    make install

# Final image
FROM alpine:edge

ENV AKASH /akash

# Install ca-certificates
RUN apk add --update ca-certificates

RUN addgroup akashuser && \
    adduser -S -G akashuser akashuser -h "$AKASH"
    
USER akashuser

WORKDIR $AKASH

# Copy over binaries from the build-env
COPY --from=build-env /go/bin/akash /usr/bin/akash

# Run akash by default, omit entrypoint to ease using container
CMD ["akash"]
