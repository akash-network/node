## Building

### Dependencies

  * [glide](https://github.com/Masterminds/glide):
  * [protocol buffers](https://developers.google.com/protocol-buffers/)
  * [protoc-gen-go](https://github.com/golang/protobuf)

#### Arch Linux:

```sh
curl https://glide.sh/get | sh
sudo pacman -Sy protobuf
go get -u github.com/golang/protobuf/protoc-gen-go
```

### Photon

Download and build photon:

```sh
go get -d github.com/ovrclk/photon
cd $GOPATH/src/github.com/ovrclk/photon
glide install
make deps
make
```
