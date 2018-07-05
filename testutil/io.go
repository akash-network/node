package testutil

import "io"

type ReadCloser struct {
	io.Reader
}

func (r ReadCloser) Close() error {
	return nil
}
