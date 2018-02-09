FROM golang:1.9
WORKDIR  /go/src/github.com/ovrclk/photon/
COPY ./demo/client .
COPY ./demo/node .
EXPOSE 46656 46657 46658
