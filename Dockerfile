FROM golang:1.9
WORKDIR  /go/src/github.com/ovrclk/photon/
COPY /demo/client .
COPY /demo/server .
RUN curl https://glide.sh/get | sh
RUN rm -rf vendor
RUN glide install
RUN cd demo && make buildamd64
EXPOSE 46656 46657 46658
