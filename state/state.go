package state

type State interface {
	Version() int64
	Hash() []byte
	Commit() ([]byte, int64, error)

	DB() DBReader
	Account() AccountAdapter
	Deployment() DeploymentAdapter
	Provider() ProviderAdapter
	Order() OrderAdapter
	DeploymentGroup() DeploymentGroupAdapter
	Fulfillment() FulfillmentAdapter
	Lease() LeaseAdapter
}

func NewState(db DB) State {
	return &state{db}
}

type state struct {
	db DB
}

func (s *state) Version() int64 {
	return s.db.Version()
}

func (s *state) Hash() []byte {
	return s.db.Hash()
}

func (s *state) Commit() ([]byte, int64, error) {
	return s.db.Commit()
}

func (s *state) DB() DBReader {
	return s.db
}

func (s *state) Account() AccountAdapter {
	return NewAccountAdapter(s.db)
}

func (s *state) Deployment() DeploymentAdapter {
	return NewDeploymentAdapter(s.db)
}

func (s *state) DeploymentGroup() DeploymentGroupAdapter {
	return NewDeploymentGroupAdapter(s.db)
}

func (s *state) Provider() ProviderAdapter {
	return NewProviderAdapter(s.db)
}

func (s *state) Order() OrderAdapter {
	return NewOrderAdapter(s.db)
}

func (s *state) Fulfillment() FulfillmentAdapter {
	return NewFulfillmentAdapter(s.db)
}

func (s *state) Lease() LeaseAdapter {
	return NewLeaseAdapter(s.db)
}
