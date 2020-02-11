package txanalyzer

type AddressDatabase interface {
	GetName(addr string) string
}
