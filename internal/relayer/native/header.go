package native

import (
	"context"
	"errors"

	cmttypes "github.com/cometbft/cometbft/types"

	"pkg.akt.dev/node/v2/internal/relayer/header"
)

func (c *Client) validators(ctx context.Context, height int64) ([]*cmttypes.Validator, error) {
	result := make([]*cmttypes.Validator, 0)
	perPage := 100

	for page := 1; page <= 10; page++ {
		reply, err := c.rpc.Validators(ctx, &height, &page, &perPage)
		if err != nil {
			return nil, err
		}
		if reply.BlockHeight != height || reply.Total < 0 || reply.Count != len(reply.Validators) {
			return nil, errors.New("validator page mismatch")
		}

		result = append(result, reply.Validators...)
		if len(result) == reply.Total {
			return result, nil
		}
	}

	return nil, errors.New("validator pagination exceeded local bound")
}

func (c *Client) HeaderSource(ctx context.Context, trusted, target int64, revision uint64) (header.Source, error) {
	if queryHeight(trusted) != nil || queryHeight(target) != nil || trusted >= maxSafeHeight {
		return header.Source{}, errors.New("invalid header heights")
	}

	commit, err := c.rpc.Commit(ctx, &target)
	if err != nil {
		return header.Source{}, err
	}
	if !commit.CanonicalCommit || commit.Header == nil || commit.Commit == nil || commit.Header.Height != target || commit.Header.ChainID != c.chainID {
		return header.Source{}, errors.New("noncanonical or mismatched Akash commit")
	}

	validators, err := c.validators(ctx, target)
	if err != nil {
		return header.Source{}, err
	}
	trustedNext, err := c.validators(ctx, trusted+1)
	if err != nil {
		return header.Source{}, err
	}

	return header.Source{
		Signed:        &commit.SignedHeader,
		Validators:    validators,
		TrustedNext:   trustedNext,
		TrustedHeight: uint64(trusted),
		Revision:      revision,
	}, nil
}
