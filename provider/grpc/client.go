package grpc

import (
	"fmt"

	mutil "github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type client struct {
	target string
}

type Client interface {
	SendManifest(*types.Manifest, txutil.Signer, *types.Provider, []byte) (*types.DeployManifestRespone, error)
}

func NewClient(target string) (Client, error) {
	return &client{target: target}, nil
}

func (c *client) SendManifest(manifest *types.Manifest, signer txutil.Signer, provider *types.Provider, deployment []byte) (*types.DeployManifestRespone, error) {
	request, err := mutil.SignManifest(manifest, signer, deployment)
	if err != nil {
		return nil, err
	}
	fmt.Printf("%v\n", c)
	fmt.Println(c.target)
	conn, err := grpc.Dial(c.target, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	// ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// defer cancel()
	client := types.NewManifestHandlerClient(conn)
	fmt.Printf("%+v", request)
	resp, err := client.DeployManifest(context.Background(), request)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
