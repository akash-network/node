package validation

import (
	"errors"
)

var (
	ErrInvalidManifest         = errors.New("invalid manifest")
	ErrManifestCrossValidation = errors.New("manifest cross validation error")
)
