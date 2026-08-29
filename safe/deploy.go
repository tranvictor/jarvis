package safe

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
)

// Official Safe singleton / factory / fallback-handler deployments.
// Addresses are CREATE2-deterministic, so the same hex is used on every
// chain that Safe deployed to. Two variants exist per 1.3.0 contract
// (canonical vs eip155); 1.4.1 ships a single canonical set.
//
// Source: https://github.com/safe-global/safe-deployments
type deployCandidate struct {
	Address string
	Label   string
}

type safeRelease struct {
	Version   string
	Factories []deployCandidate
	Safe      []deployCandidate // L1 singleton (no extra events)
	SafeL2    []deployCandidate // L2 singleton (emits extra events for indexers)
	Fallback  []deployCandidate
}

var (
	safeRelease141 = safeRelease{
		Version: "1.4.1",
		Factories: []deployCandidate{
			{"0x4e1DCf7AD4e460CfD30791CCC4F9c8a4f820ec67", "SafeProxyFactory 1.4.1 (canonical)"},
		},
		Safe: []deployCandidate{
			{"0x41675C099F32341bf84BFc5382aF534df5C7461a", "Safe 1.4.1 (canonical)"},
		},
		SafeL2: []deployCandidate{
			{"0x29fcB43b46531BcA003ddC8FCB67FFE9194148C4", "SafeL2 1.4.1 (canonical)"},
		},
		Fallback: []deployCandidate{
			{"0xfd0732Dc9E303f09fCEf3a7388Ad10A83459Ec99", "CompatibilityFallbackHandler 1.4.1 (canonical)"},
		},
	}
	safeRelease130 = safeRelease{
		Version: "1.3.0",
		Factories: []deployCandidate{
			{"0xa6B71E26C5e0845f74c812102Ca7114b6a896AB2", "GnosisSafeProxyFactory 1.3.0 (canonical)"},
			{"0xC22834581EbC8527d974F8a1c97E1bEA4EF910BC", "GnosisSafeProxyFactory 1.3.0 (eip155)"},
		},
		Safe: []deployCandidate{
			{"0xd9Db270c1B5E3Bd161E8c8503c55cEABeE709552", "GnosisSafe 1.3.0 (canonical)"},
			{"0x69f4D1788e39c87893C980c06EdF4b7f686e2938", "GnosisSafe 1.3.0 (eip155)"},
		},
		SafeL2: []deployCandidate{
			{"0x3E5c63644E683549055b9Be8653de26E0B4CD36E", "GnosisSafeL2 1.3.0 (canonical)"},
			{"0xfb1bffC9d739B8D520DaF37dF666da4C687191EA", "GnosisSafeL2 1.3.0 (eip155)"},
		},
		Fallback: []deployCandidate{
			{"0xf48f2B2d2a534e402487b3ee7C18c33Aec0Fe5e4", "CompatibilityFallbackHandler 1.3.0 (canonical)"},
			{"0x017062a1dE2FE6b99BE3d9d37841FeD19F573804", "CompatibilityFallbackHandler 1.3.0 (eip155)"},
		},
	}
)

// releasesNewestFirst is the probe order for a new Safe. 1.4.1 is what
// the Safe{Wallet} UI deploys today; 1.3.0 is the fallback on chains that
// never received the 1.4.1 set.
func releasesNewestFirst() []safeRelease {
	return []safeRelease{safeRelease141, safeRelease130}
}

// preferL2Singleton reports whether this chain should deploy SafeL2
// (extra events, needed by indexers that lack a tracing API) rather than
// the L1 Safe singleton. Ethereum mainnet and its public L1 testnets
// have tracing, so they use the cheaper L1 singleton — matching the
// Safe{Wallet} UI.
func preferL2Singleton(chainID uint64) bool {
	switch chainID {
	case 1, 11155111, 17000, 560048: // ethereum, sepolia, holesky, hoodi
		return false
	default:
		return true
	}
}

// DeployOverrides lets an operator point jarvis at their own factory /
// singleton / fallback-handler instead of the canonical Safe deployments.
// Empty fields are filled by on-chain probing.
type DeployOverrides struct {
	Factory         string
	Singleton       string
	FallbackHandler string
}

// Deployment is the fully-resolved set of contracts used to deploy one
// new Safe, plus human-readable labels for the confirmation screen.
type Deployment struct {
	Version         string
	Factory         common.Address
	FactoryLabel    string
	Singleton       common.Address
	SingletonLabel  string
	FallbackHandler common.Address
	FallbackLabel   string
}

const (
	proxyFactoryABIJSON = `[
	  {
	    "inputs": [
	      {"internalType": "address", "name": "_singleton", "type": "address"},
	      {"internalType": "bytes", "name": "initializer", "type": "bytes"},
	      {"internalType": "uint256", "name": "saltNonce", "type": "uint256"}
	    ],
	    "name": "createProxyWithNonce",
	    "outputs": [{"internalType": "address", "name": "proxy", "type": "address"}],
	    "stateMutability": "nonpayable",
	    "type": "function"
	  },
	  {
	    "inputs": [],
	    "name": "proxyCreationCode",
	    "outputs": [{"internalType": "bytes", "name": "", "type": "bytes"}],
	    "stateMutability": "pure",
	    "type": "function"
	  }
	]`

	safeSetupABIJSON = `[
	  {
	    "inputs": [
	      {"internalType": "address[]", "name": "_owners", "type": "address[]"},
	      {"internalType": "uint256", "name": "_threshold", "type": "uint256"},
	      {"internalType": "address", "name": "to", "type": "address"},
	      {"internalType": "bytes", "name": "data", "type": "bytes"},
	      {"internalType": "address", "name": "fallbackHandler", "type": "address"},
	      {"internalType": "address", "name": "paymentToken", "type": "address"},
	      {"internalType": "uint256", "name": "payment", "type": "uint256"},
	      {"internalType": "address payable", "name": "paymentReceiver", "type": "address"}
	    ],
	    "name": "setup",
	    "outputs": [],
	    "stateMutability": "nonpayable",
	    "type": "function"
	  }
	]`
)

// GetProxyFactoryABI returns the parsed SafeProxyFactory subset used for
// createProxyWithNonce and proxyCreationCode.
func GetProxyFactoryABI() *abi.ABI {
	a, err := abi.JSON(strings.NewReader(proxyFactoryABIJSON))
	if err != nil {
		panic(err)
	}
	return &a
}

// GetSafeSetupABI returns the parsed Safe.setup ABI used to build the
// initializer passed to createProxyWithNonce.
func GetSafeSetupABI() *abi.ABI {
	a, err := abi.JSON(strings.NewReader(safeSetupABIJSON))
	if err != nil {
		panic(err)
	}
	return &a
}

// EncodeSetup packs Safe.setup(...) for a standard new wallet: no
// post-setup delegatecall, no payment, and the given fallback handler.
func EncodeSetup(owners []common.Address, threshold *big.Int, fallbackHandler common.Address) ([]byte, error) {
	if err := ValidateNewSafeParams(owners, threshold); err != nil {
		return nil, err
	}
	if threshold == nil {
		return nil, fmt.Errorf("threshold is required")
	}
	return GetSafeSetupABI().Pack(
		"setup",
		owners,
		threshold,
		common.Address{},
		[]byte{},
		fallbackHandler,
		common.Address{},
		big.NewInt(0),
		common.Address{},
	)
}

// EncodeCreateProxyWithNonce packs SafeProxyFactory.createProxyWithNonce.
func EncodeCreateProxyWithNonce(singleton common.Address, initializer []byte, saltNonce *big.Int) ([]byte, error) {
	if singleton == (common.Address{}) {
		return nil, fmt.Errorf("singleton address is required")
	}
	if saltNonce == nil {
		saltNonce = big.NewInt(0)
	}
	if saltNonce.Sign() < 0 {
		return nil, fmt.Errorf("salt nonce must be non-negative")
	}
	return GetProxyFactoryABI().Pack("createProxyWithNonce", singleton, initializer, saltNonce)
}

// PredictProxyAddress is the CREATE2 address SafeProxyFactory uses for
// createProxyWithNonce: salt = keccak256(keccak256(initializer) || saltNonce),
// init code = proxyCreationCode || uint256(singleton).
func PredictProxyAddress(
	factory, singleton common.Address,
	proxyCreationCode, initializer []byte,
	saltNonce *big.Int,
) (common.Address, error) {
	if factory == (common.Address{}) {
		return common.Address{}, fmt.Errorf("factory address is required")
	}
	if singleton == (common.Address{}) {
		return common.Address{}, fmt.Errorf("singleton address is required")
	}
	if len(proxyCreationCode) == 0 {
		return common.Address{}, fmt.Errorf("proxy creation code is empty")
	}
	if saltNonce == nil {
		saltNonce = big.NewInt(0)
	}
	if saltNonce.Sign() < 0 {
		return common.Address{}, fmt.Errorf("salt nonce must be non-negative")
	}

	initHash := crypto.Keccak256(initializer)
	salt := crypto.Keccak256Hash(append(initHash, common.LeftPadBytes(saltNonce.Bytes(), 32)...))
	deploymentData := append(append([]byte{}, proxyCreationCode...), common.LeftPadBytes(singleton.Bytes(), 32)...)
	return crypto.CreateAddress2(factory, salt, crypto.Keccak256(deploymentData)), nil
}

// ValidateNewSafeParams mirrors Safe.setup's owner / threshold checks so
// we fail before the user signs a reverting factory call.
func ValidateNewSafeParams(owners []common.Address, threshold *big.Int) error {
	if len(owners) == 0 {
		return fmt.Errorf("need at least one owner")
	}
	if threshold == nil || threshold.Sign() <= 0 {
		return fmt.Errorf("threshold must be at least 1")
	}
	if threshold.Cmp(big.NewInt(int64(len(owners)))) > 0 {
		return fmt.Errorf("threshold %s is greater than owner count %d", threshold, len(owners))
	}
	seen := make(map[common.Address]struct{}, len(owners))
	for i, o := range owners {
		if o == (common.Address{}) {
			return fmt.Errorf("owner %d is the zero address", i+1)
		}
		if _, ok := seen[o]; ok {
			return fmt.Errorf("duplicate owner %s", o.Hex())
		}
		seen[o] = struct{}{}
	}
	return nil
}

// hasCodeFunc is the on-chain probe used by ResolveSafeDeployment. Tests
// inject a fake; production uses util.IsContract.
type hasCodeFunc func(address string) (bool, error)

// ResolveSafeDeployment picks factory / singleton / fallback-handler for
// a new Safe on network. Canonical 1.4.1 is preferred when those
// contracts have code; 1.3.0 (canonical then eip155) is the fallback.
// L2 chains get SafeL2 so indexers see the extra events.
func ResolveSafeDeployment(network networks.Network, overrides DeployOverrides) (*Deployment, error) {
	return resolveSafeDeployment(network.GetChainID(), func(addr string) (bool, error) {
		return util.IsContract(addr, network)
	}, overrides)
}

func resolveSafeDeployment(chainID uint64, hasCode hasCodeFunc, overrides DeployOverrides) (*Deployment, error) {
	factoryOverride, err := parseOptionalAddress(overrides.Factory, "--factory")
	if err != nil {
		return nil, err
	}
	singletonOverride, err := parseOptionalAddress(overrides.Singleton, "--singleton")
	if err != nil {
		return nil, err
	}
	fallbackOverride, err := parseOptionalAddress(overrides.FallbackHandler, "--fallback-handler")
	if err != nil {
		return nil, err
	}

	if factoryOverride != nil && singletonOverride != nil && fallbackOverride != nil {
		return &Deployment{
			Version:         "custom",
			Factory:         *factoryOverride,
			FactoryLabel:    "user-supplied via --factory",
			Singleton:       *singletonOverride,
			SingletonLabel:  "user-supplied via --singleton",
			FallbackHandler: *fallbackOverride,
			FallbackLabel:   "user-supplied via --fallback-handler",
		}, nil
	}

	wantL2 := preferL2Singleton(chainID)
	var lastErr error
	for _, rel := range releasesNewestFirst() {
		d := Deployment{Version: rel.Version}

		if factoryOverride != nil {
			d.Factory = *factoryOverride
			d.FactoryLabel = "user-supplied via --factory"
		} else if addr, label, err := firstWithCode(hasCode, rel.Factories); err == nil {
			d.Factory = addr
			d.FactoryLabel = label + ", verified on-chain"
		} else {
			lastErr = err
			continue
		}

		// The other flavour is appended so a chain that only deployed one
		// still succeeds; the preferred flavour is just tried first.
		var singletons []deployCandidate
		if wantL2 {
			singletons = append(append([]deployCandidate{}, rel.SafeL2...), rel.Safe...)
		} else {
			singletons = append(append([]deployCandidate{}, rel.Safe...), rel.SafeL2...)
		}

		if singletonOverride != nil {
			d.Singleton = *singletonOverride
			d.SingletonLabel = "user-supplied via --singleton"
		} else if addr, label, err := firstWithCode(hasCode, singletons); err == nil {
			d.Singleton = addr
			d.SingletonLabel = label + ", verified on-chain"
		} else {
			lastErr = err
			continue
		}

		if fallbackOverride != nil {
			d.FallbackHandler = *fallbackOverride
			d.FallbackLabel = "user-supplied via --fallback-handler"
		} else if addr, label, err := firstWithCode(hasCode, rel.Fallback); err == nil {
			d.FallbackHandler = addr
			d.FallbackLabel = label + ", verified on-chain"
		} else {
			lastErr = err
			continue
		}

		return &d, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no candidate had code")
	}
	return nil, fmt.Errorf(
		"couldn't locate Safe factory/singleton/fallback-handler on chain %d: %w; "+
			"pass --factory / --singleton / --fallback-handler explicitly",
		chainID, lastErr,
	)
}

func firstWithCode(hasCode hasCodeFunc, candidates []deployCandidate) (common.Address, string, error) {
	tried := make([]string, 0, len(candidates))
	for _, c := range candidates {
		tried = append(tried, c.Address)
		ok, err := hasCode(c.Address)
		if err != nil || !ok {
			continue
		}
		return common.HexToAddress(c.Address), c.Label, nil
	}
	return common.Address{}, "", fmt.Errorf("none of %s have code", strings.Join(tried, ", "))
}

func parseOptionalAddress(raw, flag string) (*common.Address, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !common.IsHexAddress(raw) {
		return nil, fmt.Errorf("%s %q is not a valid address", flag, raw)
	}
	addr := common.HexToAddress(raw)
	return &addr, nil
}

// FactoryProxyCreationCode reads factory.proxyCreationCode() so the
// CREATE2 address can be predicted before the user signs.
func FactoryProxyCreationCode(factory common.Address, network networks.Network) ([]byte, error) {
	r, err := util.EthReader(network)
	if err != nil {
		return nil, err
	}
	var code []byte
	if err := r.ReadContractWithABI(&code, factory.Hex(), GetProxyFactoryABI(), "proxyCreationCode"); err != nil {
		return nil, err
	}
	if len(code) == 0 {
		return nil, fmt.Errorf("factory %s returned empty proxyCreationCode", factory.Hex())
	}
	return code, nil
}
