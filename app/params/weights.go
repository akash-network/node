package params

// Default simulation operation weights for messages and gov proposals
const (
	DefaultWeightMsgCreateProvider int = 100
	DefaultWeightMsgUpdateProvider int = 5

	DefaultWeightMsgCreateDeployment int = 100
	DefaultWeightMsgUpdateDeployment int = 10
	DefaultWeightMsgCloseDeployment  int = 100
	DefaultWeightMsgCloseGroup       int = 100

	DefaultWeightMsgCreateBid  int = 100
	DefaultWeightMsgCloseBid   int = 100
	DefaultWeightMsgCloseLease int = 10
)
