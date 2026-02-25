package keys

var (
	// Old prefixes — kept for migration reads
	DeploymentPrefix = []byte{0x11, 0x00}
	GroupPrefix      = []byte{0x12, 0x00}

	// New collections prefixes
	DeploymentPrefixNew        = []byte{0x11, 0x01}
	DeploymentIndexStatePrefix = []byte{0x11, 0x02}
	GroupPrefixNew             = []byte{0x12, 0x01}
	GroupIndexStatePrefix      = []byte{0x12, 0x02}
	GroupIndexDeploymentPrefix = []byte{0x12, 0x03}
)
