FROM golang:1.9
WORKDIR  /go/src/github.com/ovrclk/photon/
COPY ./photon .
COPY ./photond .
EXPOSE 46656 46657 46658
