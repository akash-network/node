FROM golang:1.10-alpine as builder
WORKDIR  /go/src/github.com/ovrclk/photon/
COPY . .
RUN apk --update add curl git build-base linux-headers
RUN curl https://glide.sh/get | sh
RUN glide install
RUN go build ./cmd/photond
RUN go build ./cmd/photon

FROM alpine:3.7
WORKDIR /
COPY --from=builder                        \
  /go/src/github.com/ovrclk/photon/photond \
  /go/src/github.com/ovrclk/photon/photon  \
  /
EXPOSE 46656 46657 46658
