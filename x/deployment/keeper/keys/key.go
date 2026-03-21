package keys

var (
	// Old prefixes — kept for migration reads
	DeploymentPrefixLegacy = []byte{0x11, 0x00}
	GroupPrefixLegacy      = []byte{0x12, 0x00}

	// New collections prefixes
	DeploymentPrefix           = []byte{0x11, 0x01}
	DeploymentIndexStatePrefix = []byte{0x11, 0x02}
	GroupPrefix                = []byte{0x12, 0x01}
	GroupIndexStatePrefix      = []byte{0x12, 0x02}
	GroupIndexDeploymentPrefix = []byte{0x12, 0x03}

	// Pending denom migration prefix
	PendingDenomMigrationPrefix = []byte{0x13, 0x01}

	ParamsKey = []byte{0x14, 0x00} // key for deployment module params
)
