package cli

import (
	"errors"
)

var (
	errInvalidSerialFlag = errors.New("invalid value in serial flag. expected integer")
)
