package clientcommon

import (
	"fmt"
	"github.com/tendermint/tendermint/libs/log"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/homedir"
	"os"
	"path"
)

func OpenKubeConfig(cfgPath string, log log.Logger) (*rest.Config, error) {
	// If no value is specified, use a default
	if len(cfgPath) == 0 {
		cfgPath = path.Join(homedir.HomeDir(), ".kube", "config")
	}

	// Always bypass the default rate limiting
	rateLimiter := flowcontrol.NewFakeAlwaysRateLimiter()

	if _, err := os.Stat(cfgPath); err == nil {
		log.Info("using kube config file", "path", cfgPath)
		cfg, err := clientcmd.BuildConfigFromFlags("", cfgPath)
		if err != nil {
			return cfg, fmt.Errorf("%w: error building kubernetes config", err)
		}
		cfg.RateLimiter = rateLimiter
		return cfg, err
	}

	log.Info("using in cluster kube config")
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return cfg, fmt.Errorf("%w: error building kubernetes config", err)
	}
	cfg.RateLimiter = rateLimiter

	return cfg, err
}
