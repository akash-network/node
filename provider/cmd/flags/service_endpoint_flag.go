package flags

import (
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/libs/log"
	"net"
	"strconv"
	"strings"
)

var (
	errInvalidEndpointFlagValue = errors.New("invalid endpoint flag value")
)

func AddServiceEndpointFlag(cmd *cobra.Command, serviceName string) error {
	flagName := fmt.Sprintf("%s-endpoint", serviceName)
	cmd.Flags().String(flagName, "", fmt.Sprintf("host & port for %s (empty for autodetect)", serviceName))
	if err := viper.BindPFlag(flagName, cmd.Flags().Lookup(flagName)); err != nil {
		return err
	}

	return nil
}

func GetServiceEndpointFlagValue(logger log.Logger, serviceName string) (*net.SRV, error) {
	flagName := fmt.Sprintf("%s-endpoint", serviceName)
	flagValue := viper.GetString(flagName)
	if len(flagValue) == 0 {
		logger.Debug("service being found via autodetection", "service", serviceName)
		return nil, nil
	}

	logger.Debug("parsing endpoint", "service", serviceName, "value", flagValue)

	flagValueParts := strings.SplitN(flagValue, ":", 2)
	if len(flagValueParts) != 2 {
		return nil, fmt.Errorf("%w: endpoint for service %q is missing port", errInvalidEndpointFlagValue, serviceName)
	}

	port, err := strconv.ParseUint(flagValueParts[1], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("endpoint for service %q has invalid port %q: %w", serviceName, flagValueParts[1], err)
	}

	host := flagValueParts[0]
	if len(host) == 0 {
		return nil, fmt.Errorf("%w: endpoint for %q is missing host", errInvalidEndpointFlagValue, serviceName)
	}

	result := net.SRV{
		Target:   flagValueParts[0],
		Port:     uint16(port),
		Priority: 0,
		Weight:   0,
	}

	logger.Debug("using manually configured endpoint", "service", serviceName, "host", result.Target, "port", result.Port)
	return &result, nil
}
