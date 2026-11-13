package e2e

const (
	providerBinary   = "provider"
	consumerBinary   = "consumer"
	// consumerBech32Prefix is the address prefix the consumer chain runs
	// under. It differs from the provider's ("cosmos", this test binary's own
	// configuration) on purpose: every scenario then crosses the two prefixes,
	// so an address that leaks across the wire as a bech32 string fails here.
	// Consumer-side addresses are re-encoded under this binary's prefix when
	// they are read (see queryConsumerNetValidators and consAddrToCometHex).
	consumerBech32Prefix = "shard"
	providerHomePath = "/home/nonroot/.provider"
	consumerHomePath = "/home/nonroot/.consumer"
	bondDenom        = "uatone"
	feeDenom         = "uphoton"
	providerChainID  = "provider-e2e"
	consumerChainID  = "consumer-e2e"
	// initHomesPath is where an init container sees the host directory that
	// holds a chain's node homes (val0, val1, ...).
	initHomesPath = "/home/nonroot/homes"
)

// chain represents a Cosmos chain instance (either provider or consumer).
// Configuration is managed via Docker exec commands against the chain binary.
type chain struct {
	// dataDir is the host home directory of val0, the node the suite drives.
	dataDir string
	// homes is the host home directory of every validator node, by index;
	// homes[0] is dataDir.
	homes []string
	id    string
}
