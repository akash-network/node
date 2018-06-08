package bidengine

import "github.com/ovrclk/akash/types"

func matchProviderAttributes(
	pattrs []types.ProviderAttribute,
	reqs []types.ProviderAttribute) bool {

	for _, req := range reqs {
		found := false
		for _, attr := range pattrs {
			if req.Name != attr.Name {
				continue
			}
			if req.Value != attr.Value {
				return false
			}
			found = true
			break
		}
		if !found {
			return false
		}
	}
	return true
}
