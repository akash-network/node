package types

type LeaseIPStatus struct {
	Port         uint32
	ExternalPort uint32
	ServiceName  string
	IP           string
	Protocol     string
}
