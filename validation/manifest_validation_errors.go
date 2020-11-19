package validation

import (
	"errors"
)

var (
	ErrInvalidManifest = errors.New("invalid manifest")
	ErrServiceExposePortZero = errors.New("The service port is zero")
 	ErrManifestCrossValidation = errors.New("manifest cross validation error")
)
