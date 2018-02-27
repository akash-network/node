package state

type State interface {
	Version() uint64
	Hash() []byte
	Commit(version uint64) ([]byte, error)

	DB() DBReader

	Account() AccountAdapter
	Deployment() DeploymentAdapter
	Datacenter() DatacenterAdapter
}

func NewState(db DB) State {
	return &state{db}
}

type state struct {
	db DB
}

func (s *state) Version() uint64 {
	return s.db.Version()
}

func (s *state) Hash() []byte {
	return s.db.Hash()
}

func (s *state) Commit(version uint64) ([]byte, error) {
	return s.db.Commit(version)
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

func (s *state) Datacenter() DatacenterAdapter {
	return NewDatacenterAdapter(s.db)
}
