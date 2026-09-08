package util

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
)

// decodeSigningCalldata runs the analyzer over an inner call for Classic and
// Safe signing cards. extraABIs (Safe MultiSend destinations) win over the
// explorer ABI. Proxy contracts are followed to their implementation by
// GetABI / ConfigToABI. Returns nil when there is no analyzer or no data.
func decodeSigningCalldata(
	to string,
	value *big.Int,
	data []byte,
	network jarvisnetworks.Network,
	resolver ABIResolver,
	analyzer util.TxAnalyzer,
	extraABIs map[string]*abi.ABI,
) *jarviscommon.FunctionCall {
	if analyzer == nil || len(data) == 0 {
		return nil
	}
	customABIs := map[string]*abi.ABI{}
	for addr, a := range extraABIs {
		customABIs[strings.ToLower(addr)] = a
	}
	if resolver != nil {
		if destAbi, err := resolver.ConfigToABI(to, config.ForceERC20ABI, config.CustomABI, network); err == nil {
			key := strings.ToLower(to)
			if _, taken := customABIs[key]; !taken {
				customABIs[key] = destAbi
			}
		}
	}
	return analyzer.AnalyzeFunctionCallRecursively(lookupABI(resolver), value, to, data, customABIs)
}

func lookupABI(resolver ABIResolver) jarviscommon.ABIDatabase {
	if resolver != nil {
		return resolver.GetABI
	}
	return util.GetABI
}
