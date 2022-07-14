package util

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"io"
	"regexp"
	"strings"
)

var allowedIpEndpointNameRegex = regexp.MustCompile(`^[a-z0-9\-]+$`)

func MakeIPSharingKey(lID mtypes.LeaseID, endpointName string) string {

	effectiveName := endpointName
	if !allowedIpEndpointNameRegex.MatchString(endpointName) {
		h := sha256.New()
		_, err := io.WriteString(h, endpointName)
		if err != nil {
			panic(err)
		}
		effectiveName = strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(h.Sum(nil)[0:15]))
	}
	return fmt.Sprintf("%s-ip-%s", lID.GetOwner(), effectiveName)
}
