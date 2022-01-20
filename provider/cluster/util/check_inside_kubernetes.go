package util

import (
	"os"
	"sync"
)

const (
	kubeSecretPath = "/var/run/secrets/kubernetes.io" // nolint:gosec
)

var insideKubernetes bool
var checkedForKubernetes bool

var kubeCheckLock sync.Mutex

func IsInsideKubernetes() (bool, error) {
	kubeCheckLock.Lock()
	defer kubeCheckLock.Unlock()

	if checkedForKubernetes {
		return insideKubernetes, nil
	}

	// Check if running in kubernetes, or if running external to kubernetes
	_, err := os.Stat(kubeSecretPath)

	if err != nil {
		if os.IsNotExist(err) { // Does not exist, so not inside kubernetes
			insideKubernetes = false
			checkedForKubernetes = true
			return insideKubernetes, nil
		}

		return false, err
	}

	// Exists, so we're in kubernetes
	insideKubernetes = true
	checkedForKubernetes = true
	return insideKubernetes, nil
}
