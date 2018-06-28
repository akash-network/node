package grpc

import (
	"github.com/ovrclk/akash/types"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type client struct {
	target string
}

type Client interface {
	Deploy(context.Context, *types.ManifestRequest) (*types.DeployRespone, error)
}

func NewClient(target string) (Client, error) {
	return &client{target: target}, nil
}

func (c *client) Deploy(ctx context.Context, manifestRequest *types.ManifestRequest) (*types.DeployRespone, error) {
	conn, err := grpc.Dial(c.target, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	client := types.NewClusterClient(conn)
	resp, err := client.Deploy(ctx, manifestRequest)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
