FROM golang:1.10-alpine as builder
WORKDIR  /go/src/github.com/ovrclk/akash/
COPY . .
RUN apk --update add curl git build-base linux-headers
RUN curl https://glide.sh/get | sh
RUN glide install
RUN go build ./cmd/akashd
RUN go build ./cmd/akash

FROM alpine:3.7
WORKDIR /
COPY --from=builder                        \
  /go/src/github.com/ovrclk/akash/akashd \
  /go/src/github.com/ovrclk/akash/akash  \
  /
EXPOSE 46656 46657 46658
