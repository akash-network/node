package state

type CommitState interface {
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

func NewState(db DB) CommitState {
	return &commitState{db}
}

type commitState struct {
	db DB
}

func (s *commitState) Version() int64 {
	return s.db.Version()
}

func (s *commitState) Hash() []byte {
	return s.db.Hash()
}

func (s *commitState) Commit() ([]byte, int64, error) {
	return s.db.Commit()
}

func (s *commitState) DB() DBReader {
	return s.db
}

func (s *commitState) Account() AccountAdapter {
	return NewAccountAdapter(s.db)
}

func (s *commitState) Deployment() DeploymentAdapter {
	return NewDeploymentAdapter(s.db)
}

func (s *commitState) DeploymentGroup() DeploymentGroupAdapter {
	return NewDeploymentGroupAdapter(s.db)
}

func (s *commitState) Provider() ProviderAdapter {
	return NewProviderAdapter(s.db)
}

func (s *commitState) Order() OrderAdapter {
	return NewOrderAdapter(s.db)
}

func (s *commitState) Fulfillment() FulfillmentAdapter {
	return NewFulfillmentAdapter(s.db)
}

func (s *commitState) Lease() LeaseAdapter {
	return NewLeaseAdapter(s.db)
}
