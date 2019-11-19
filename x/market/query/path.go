package query

const (
	ordersPath = "orders"
	bidsPath   = "bids"
	leasesPath = "leases"
)

func OrdersPath() string {
	return ordersPath
}

func BidsPath() string {
	return bidsPath
}

func LeasesPath() string {
	return leasesPath
}
