package types

type Endpoints []Endpoint

func (m Endpoints) Dup() Endpoints {
	res := make(Endpoints, len(m))

	copy(res, m)

	return res
}
