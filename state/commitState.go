package state

type CommitState interface {
	Version() int64
	Hash() []byte
	Commit() ([]byte, int64, error)
	Get(key []byte) []byte
	GetRange([]byte, []byte, int) ([][]byte, [][]byte, error)
	Set(key, val []byte)
	Remove(key []byte)

	Account() AccountAdapter
	Deployment() DeploymentAdapter
	Provider() ProviderAdapter
	Order() OrderAdapter
	DeploymentGroup() DeploymentGroupAdapter
	Fulfillment() FulfillmentAdapter
	Lease() LeaseAdapter
}

func NewState(db DB) CommitState {
	return &commitState{db}
}

type commitState struct {
	db DB
}

func (c *commitState) Version() int64 {
	return c.db.Version()
}

func (c *commitState) Hash() []byte {
	return c.db.Hash()
}

func (c *commitState) Commit() ([]byte, int64, error) {
	return c.db.Commit()
}

func (c *commitState) DB() DBReader {
	return c.db
}

func (c *commitState) Get(key []byte) []byte {
	return c.db.Get(key)
}

func (c *commitState) GetRange(start, end []byte, limit int) ([][]byte, [][]byte, error) {
	return c.db.GetRange(start, end, limit)
}

func (c *commitState) Remove(key []byte) {
	c.db.Remove(key)
}

func (c *commitState) Set(key, val []byte) {
	c.db.Set(key, val)
}

func (c *commitState) Account() AccountAdapter {
	return NewAccountAdapter(c)
}

func (c *commitState) Deployment() DeploymentAdapter {
	return NewDeploymentAdapter(c)
}

func (c *commitState) DeploymentGroup() DeploymentGroupAdapter {
	return NewDeploymentGroupAdapter(c)
}

func (c *commitState) Provider() ProviderAdapter {
	return NewProviderAdapter(c)
}

func (c *commitState) Order() OrderAdapter {
	return NewOrderAdapter(c)
}

func (c *commitState) Fulfillment() FulfillmentAdapter {
	return NewFulfillmentAdapter(c)
}

func (c *commitState) Lease() LeaseAdapter {
	return NewLeaseAdapter(c)
}
