package v2_1_0

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const (
	pythContractAddr             = "akash1nc5tatafv6eyq7llkr2gv50ff9e22mnf70qgjlv737ktmt4eswrqyagled"
	pythProRouterSetIndex        = 0
	pythProExpectedEmitterChain  = 26
	pythProExpectedEmitterHex    = "507974686e6574507974686e6574507974686e6574507974686e657450797468"
	pythProVerifierContractLabel = "pyth-pro-verifier"
)

var pythProRoutersHex = []string{
	"41534bb176e461a3fb30479400f210549ecce638",
	"6502987b62f21cab7eb5ccd8f0173084b60d5b41",
	"44a3e8f6a382412cf6bb90a3f8106e68977476c9",
	"d9d7d4529577864352c9a6539a48238fcd447052",
	"1663a5a822336ece48559b1dfb1e93a017a7dac3",
}

type pythProVerifierInstantiateMsg struct {
	RouterSetIndex         uint32         `json:"router_set_index"`
	Routers                []routerConfig `json:"routers"`
	ExpectedEmitterChain   uint16         `json:"expected_emitter_chain"`
	ExpectedEmitterAddress []byte         `json:"expected_emitter_address"`
}

type routerConfig struct {
	Bytes []byte `json:"bytes"`
}

type pythMigrateMsg struct {
	WormholeContract string `json:"wormhole_contract,omitempty"`
}

func newPythProVerifierInstantiateMsg() ([]byte, error) {
	routers := make([]routerConfig, 0, len(pythProRoutersHex))
	for _, router := range pythProRoutersHex {
		routerBytes, err := decodeHex(router)
		if err != nil {
			return nil, fmt.Errorf("failed to decode pyth router address: %w", err)
		}
		routers = append(routers, routerConfig{Bytes: routerBytes})
	}

	emitter, err := decodeHex(pythProExpectedEmitterHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode pyth emitter address: %w", err)
	}

	msg := pythProVerifierInstantiateMsg{
		RouterSetIndex:         pythProRouterSetIndex,
		Routers:                routers,
		ExpectedEmitterChain:   pythProExpectedEmitterChain,
		ExpectedEmitterAddress: emitter,
	}

	return json.Marshal(msg)
}

func newPythVerifierMigrationMsg(verifierAddr string) ([]byte, error) {
	return json.Marshal(pythMigrateMsg{WormholeContract: verifierAddr})
}

func decodeHex(value string) ([]byte, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}
