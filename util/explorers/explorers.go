package explorers

type BlockExplorer interface {
	RecommendedGasPrice() (float64, error)
	GetABIString(address string) (string, error)
}
