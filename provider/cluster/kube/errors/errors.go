package errors

import (
	"errors"
	"fmt"
)

var (
	ErrKubeClient                = errors.New("kube")
	ErrInternalError             = fmt.Errorf("%w: internal error", ErrKubeClient)
	ErrLeaseNotFound             = fmt.Errorf("%w: lease not found", ErrKubeClient)
	ErrNoDeploymentForLease      = fmt.Errorf("%w: no deployments for lease", ErrKubeClient)
	ErrNoManifestForLease        = fmt.Errorf("%w: no manifest for lease", ErrKubeClient)
	ErrNoServiceForLease         = fmt.Errorf("%w: no service for that lease", ErrKubeClient)
	ErrInvalidHostnameConnection = fmt.Errorf("%w: invalid hostname connection", ErrKubeClient)
	ErrNotConfiguredWithSettings = fmt.Errorf("%w: not configured with settings in the context passed to function", ErrKubeClient)
	ErrAlreadyExists             = fmt.Errorf("%w: resource already exists", ErrKubeClient)
)
