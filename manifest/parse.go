package manifest

import (
	"errors"
	"strings"
)

var ErrUnsupportedServiceProtocol = errors.New("Unsuported service protocol")

func ParseServiceProtocol(input string) (ServiceProtocol, error) {
	var result ServiceProtocol

	// This is not a case sensitive parse, so make all input
	// uppercase
	input = strings.ToUpper(input)

	switch input {
	case "TCP", "": // The empty string (no input) implies TCP
		result = TCP
	case "UDP":
		result = UDP
	default:
		return result, ErrUnsupportedServiceProtocol
	}

	return result, nil
}
