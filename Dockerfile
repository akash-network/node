FROM golang:1.9
WORKDIR  /go/src/github.com/ovrclk/photon/
RUN export GOPATH="/go/src/x/y/z/vendor:/go"
COPY . .
RUN curl https://glide.sh/get | sh
RUN rm -rf vendor
RUN glide install
RUN cd demo && make dockerbuild
