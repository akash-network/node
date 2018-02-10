FROM golang:1.9
WORKDIR  /go/src/github.com/ovrclk/photon/
COPY . .
RUN curl https://glide.sh/get | sh
RUN rm -rf vendor
RUN glide install
RUN make buildamd64
EXPOSE 46656 46657 46658
