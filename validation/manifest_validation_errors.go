package validation

import (
	"errors"

	)

var ErrGroupContainsNoServices = errors.New("The deployment group contains 0 services")
var ErrServiceNameEmpty = errors.New("The service name is the empty string")
var ErrServiceImageEmpty = errors.New("The service image naem is empty")
var ErrServiceEnvVarEmptyName = errors.New("An environmental variable has an empty name")
var ErrServiceCountIsZero = errors.New("The service count is zero")
var ErrServiceExposePortZero = errors.New("The service port is zero")
var ErrServiceExposeInvalidHostname = errors.New("The service hostname is not valid")
var ErrManifestGroupDoesNotExistInDeployment = errors.New("")
