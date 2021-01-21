package cli

import (
	"github.com/pkg/errors"
)

var (
	errInvalidSerialFlag = errors.New("invalid value in serial flag. expected integer")
)
