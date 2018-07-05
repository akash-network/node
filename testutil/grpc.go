package testutil

import (
	"context"

	"google.golang.org/grpc/metadata"
)

type MockGRPCStreamServer struct{}

func (s MockGRPCStreamServer) SetHeader(metadata.MD) error {
	return nil
}

func (s MockGRPCStreamServer) SendHeader(metadata.MD) error {
	return nil
}

func (s MockGRPCStreamServer) SetTrailer(metadata.MD) {

}

func (s MockGRPCStreamServer) Context() context.Context {
	return context.TODO()
}

func (s MockGRPCStreamServer) SendMsg(m interface{}) error {
	return nil
}

func (s MockGRPCStreamServer) RecvMsg(m interface{}) error {
	return nil
}
