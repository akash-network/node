package params

// Default simulation operation weights for messages and gov proposals
const (
	DefaultWeightMsgCreateValidator int = 100
	DefaultWeightMsgEditValidator   int = 5

	DefaultWeightMsgCreateDeployment int = 100
	DefaultWeightMsgUpdateDeployment int = 10
	DefaultWeightMsgCloseDeployment  int = 100

	DefaultWeightMsgCreateBid  int = 100
	DefaultWeightMsgCloseBid   int = 100
	DefaultWeightMsgCloseOrder int = 10
)
