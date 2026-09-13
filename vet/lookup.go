package vet

import (
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/util/explorers"
	"github.com/tranvictor/jarvis/util/reader"
)

// ExplorerLookup fetches verified source and proxy implementations.
type ExplorerLookup struct {
	Explorer explorers.BlockExplorer
	Reader   *reader.EthReader
}

func (l ExplorerLookup) Source(addr string) (Source, error) {
	if l.Explorer == nil {
		return Source{}, nil
	}
	src, err := l.Explorer.GetVerifiedSource(addr)
	if err != nil {
		return Source{}, err
	}
	return Source{
		Address:        src.Address,
		Code:           src.Source,
		Verified:       src.Verified,
		Implementation: src.Implementation,
		IsProxy:        src.IsProxy,
	}, nil
}

func (l ExplorerLookup) HasCode(addr string) (bool, error) {
	if l.Reader == nil {
		return true, nil
	}
	code, err := l.Reader.GetCode(addr)
	if err != nil {
		return false, err
	}
	return len(code) > 0, nil
}

func (l ExplorerLookup) Implementation(addr string) (string, error) {
	if l.Reader != nil {
		impl, err := l.Reader.ImplementationOf(-1, addr)
		if err == nil && impl != (common.Address{}) {
			return impl.Hex(), nil
		}
	}
	if l.Explorer != nil {
		info, err := l.Explorer.GetContractInfo(addr)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(info.Implementation), nil
	}
	return "", nil
}
