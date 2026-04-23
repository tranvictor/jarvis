package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"

	bleve "github.com/tranvictor/jarvis/bleve"
	jarviscommon "github.com/tranvictor/jarvis/common"
	db "github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util/addrbook"
	"github.com/tranvictor/jarvis/util/broadcaster"
	"github.com/tranvictor/jarvis/util/cache"
	"github.com/tranvictor/jarvis/util/ens"
	"github.com/tranvictor/jarvis/util/monitor"
	"github.com/tranvictor/jarvis/util/reader"
)

const (
	ETH_ADDR                  string = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	MAX_ADDR                  string = "0xffffffffffffffffffffffffffffffffffffffff"
	MIN_ADDR                  string = "0x00000000000000ffffffffffffffffffffffffff"
	ETHEREUM_MAINNET_NODE_VAR string = "ETHEREUM_MAINNET_NODE"
	ETHEREUM_ROPSTEN_NODE_VAR string = "ETHEREUM_ROPSTEN_NODE"
	TOMO_MAINNET_NODE_VAR     string = "TOMO_MAINNET_NODE"
	ETHEREUM_KOVAN_NODE_VAR   string = "ETHEREUM_KOVAN_NODE"
	ETHEREUM_RINKEBY_NODE_VAR string = "ETHEREUM_RINKEBY_NODE"
	BSC_MAINNET_NODE_VAR      string = "BSC_MAINNET_NODE"
	BSC_TESTNET_NODE_VAR      string = "BSC_TESTNET_NODE"
	ETHERSCAN_API_KEY_VAR     string = "ETHERSCAN_API_KEY"
	BSCSCAN_API_KEY_VAR       string = "BSCSCAN_API_KEY"
)

func CalculateTimeDurationFromBlock(network networks.Network, from, to uint64) time.Duration {
	if from >= to {
		return time.Duration(0)
	}
	return time.Duration(uint64(time.Second) * (to - from) * uint64(network.GetBlockTime()))
}

func GetExactAddressFromDatabases(str string) (addrs []string, names []string, scores []int) {
	addrDescs1, scores1 := bleve.GetAddresses(str)
	for i, addr := range addrDescs1 {
		addrs = append(addrs, addr.Address)
		names = append(names, addr.Desc)
		scores = append(scores, scores1[i])
	}
	return addrs, names, scores
}

func getRelevantAddressesFromDatabases(str string) (addrs []string, names []string, scores []int) {
	addrDescs1, scores1 := bleve.GetAddresses(str)
	addrDescs2, scores2 := db.GetAddresses(str)
	buffer := map[string]bool{}
	for i, addr := range addrDescs1 {
		addrs = append(addrs, addr.Address)
		names = append(names, addr.Desc)
		scores = append(scores, scores1[i])
		buffer[strings.ToLower(addr.Address)] = true
	}
	for i, addr := range addrDescs2 {
		if !buffer[strings.ToLower(addr.Address)] {
			addrs = append(addrs, addr.Address)
			names = append(names, addr.Desc)
			scores = append(scores, scores2[i])
		}
	}
	return addrs, names, scores
}

func getRelevantAddressFromDatabases(str string) (addr string, name string, err error) {
	addrs, names, _ := getRelevantAddressesFromDatabases(str)
	if len(addrs) == 0 {
		return "", "", fmt.Errorf("no address was found for '%s'", str)
	}
	return addrs[0], names[0], nil
}

func GetMatchingAddresses(str string) (addrs []string, names []string, scores []int) {
	// ENS short-circuit: when the input is a .eth name, don't dilute
	// results with fuzzy address-book matches — the resolved address is
	// the single authoritative answer, and any other hits would be a
	// coincidence on the unrelated label search.
	if a, n, ok := tryResolveENS(str); ok {
		return []string{a}, []string{n}, []int{1000}
	}
	addrs, names, scores = getRelevantAddressesFromDatabases(str)
	return addrs, names, scores
}

func GetMatchingAddress(str string) (addr string, name string, err error) {
	if a, n, ok := tryResolveENS(str); ok {
		return a, n, nil
	}
	return getRelevantAddressFromDatabases(str)
}

func GetAddressFromString(str string) (addr string, name string, err error) {
	if a, n, ok := tryResolveENS(str); ok {
		return a, n, nil
	}
	addr, name, err = getRelevantAddressFromDatabases(str)
	if err != nil {
		name = "Unknown"
		addresses := ScanForAddresses(str)
		if len(addresses) == 0 {
			return "", "", fmt.Errorf("address not found for \"%s\"", str)
		}
		addr = addresses[0]
	}
	return addr, name, nil
}

// ENS resolver wiring. We build a mainnet-only reader lazily so jarvis
// runs with no ENS-related cost for users who never type a .eth name.
// The resolver construction is also tolerant of missing mainnet node
// configs — in that case ens stays disabled, we warn once on first
// attempted resolution, and every call site falls through to its
// pre-ENS behavior.
var (
	ensResolverOnce sync.Once
	ensResolver     ens.Resolver
	ensWarnedMu     sync.Mutex
	ensWarnedBuild  bool
)

func getENSResolver() ens.Resolver {
	ensResolverOnce.Do(func() {
		r, err := EthReader(networks.EthereumMainnet)
		if err != nil {
			ensWarnedMu.Lock()
			defer ensWarnedMu.Unlock()
			if !ensWarnedBuild {
				fmt.Fprintf(
					os.Stderr,
					"warning: ENS disabled — couldn't build mainnet reader (%s). "+
						"Configure ~/.jarvis/nodes/mainnet.json or ETHEREUM_MAINNET_NODE to enable .eth name resolution.\n",
					err,
				)
				ensWarnedBuild = true
			}
			return
		}
		ensResolver = ens.NewMainnetResolver(r)
	})
	return ensResolver
}

// tryResolveENS attempts to treat str as a .eth name. It returns the
// resolved address and a display label ("ens:alice.eth") when
// successful, or ok=false when the input isn't an ENS name or
// resolution failed. Failures that actually look like ENS names (not
// just "input didn't match the pattern") emit a single stderr warning
// so the user is never silently left wondering why their .eth name
// wasn't honored.
func tryResolveENS(str string) (addr, name string, ok bool) {
	if !ens.IsLikelyENSName(str) {
		return "", "", false
	}
	r := getENSResolver()
	if r == nil {
		return "", "", false
	}
	a, err := r.Resolve(str)
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"warning: ENS resolution of %q failed (%s); falling back to address book / hex scan\n",
			str, err,
		)
		return "", "", false
	}
	label := "ens:" + strings.ToLower(strings.TrimSpace(str))
	return a.Hex(), label, true
}

func ParamToBigInt(param string) (*big.Int, error) {
	var result *big.Int
	param = strings.Trim(param, " ")
	if len(param) > 2 && param[0:2] == "0x" {
		result = jarviscommon.HexToBig(param)
	} else {
		idInt, err := strconv.Atoi(param)
		if err != nil {
			return nil, err
		}
		result = big.NewInt(int64(idInt))
	}
	return result, nil
}

// Split value by space,
// if the lowercase of first element is 'all', the amount will be "ALL", indicating a balance query is needed
// else, return the string as the amount.
// Join whats left by space and trim by space, if it is empty, interpret it
// as ETH.
// Error will not be nil if it fails to proceed all of above steps.
func ValueToAmountAndCurrency(value string) (string, string, error) {
	parts := strings.Split(value, " ")
	if len(parts) == 0 {
		return "", "", fmt.Errorf("`%s` is invalid. See help to learn more", value)
	}
	amountStr := parts[0]
	currency := strings.Trim(strings.Join(parts[1:], " "), " ")
	if len(currency) == 0 {
		currency = ETH_ADDR
	}

	if strings.ToLower(strings.Trim(amountStr, " ")) == "all" {
		return "ALL", currency, nil
	}

	return amountStr, currency, nil
}

func ScanForTxs(para string) []string {
	re := regexp.MustCompile("(0x)?[0-9a-fA-F]{64}")
	result := re.FindAllString(para, -1)
	if result == nil {
		return []string{}
	}
	return result
}

func ScanForAddresses(para string) []string {
	re := regexp.MustCompile("0x[0-9a-fA-F]{40}([^0-9a-fA-F]|$)")
	result := re.FindAllString(para, -1)
	if result == nil {
		return []string{}
	}
	for i := 0; i < len(result); i++ {
		result[i] = result[i][0:42]
	}
	return result
}

func IsAddress(addr string) bool {
	_, err := PathToAddress(addr)
	return err == nil
}

func PathToAddress(path string) (string, error) {
	re := regexp.MustCompile("(0x)?[0-9a-fA-F]{40}")
	result := re.FindAllString(path, -1)
	if result == nil {
		return "", fmt.Errorf("invalid filename")
	}
	return result[0], nil
}

func DisplayBroadcastedTx(u ui.UI, t *types.Transaction, broadcasted bool, err error, network networks.Network) {
	if !broadcasted {
		u.Error("Couldn't broadcast to any RPC for network %q. The transaction was already signed; rejections come from your node(s), not from Jarvis.", network.GetName())
		u.Error("Per node:")
		u.Error("%s", err)
		u.Info("Check each URL with your chain id (e.g. cast chain-id --rpc-url <url>). Remove or fix nodes that return the wrong chain.")
	} else {
		u.Critical("BROADCASTED TX: %s:%s", network.GetName(), t.Hash().Hex())
	}
}

func DisplayWaitAnalyze(
	u ui.UI,
	reader reader.Reader,
	analyzer TxAnalyzer,
	t *types.Transaction,
	broadcasted bool,
	err error,
	network networks.Network,
	a *abi.ABI,
	customABIs map[string]*abi.ABI,
	degenMode bool,
) {
	DisplayBroadcastedTx(u, t, broadcasted, err, network)
	if broadcasted {
		mo, err := EthTxMonitor(network)
		if err != nil {
			u.Error("Couldn't monitor the tx: %s", err)
			return
		}
		mo.BlockingWait(t.Hash().Hex())
		AnalyzeAndPrint(
			u,
			reader,
			analyzer,
			t.Hash().Hex(),
			network,
			false,
			"",
			a,
			customABIs,
			degenMode,
		)
	}
}

func AnalyzeMethodCallAndPrint(
	u ui.UI,
	analyzer TxAnalyzer,
	value *big.Int,
	destination string,
	data []byte,
	customABIs map[string]*abi.ABI,
	network networks.Network,
) (fc *jarviscommon.FunctionCall) {
	fc = analyzer.AnalyzeFunctionCallRecursively(
		GetABI, value, destination, data, customABIs)
	DisplayFunctionCall(u, fc)
	return fc
}

func AnalyzeAndPrint(
	u ui.UI,
	reader reader.Reader,
	analyzer TxAnalyzer,
	tx string,
	network networks.Network,
	forceERC20ABI bool,
	customABI string,
	a *abi.ABI,
	customABIs map[string]*abi.ABI,
	degenMode bool,
) *TxDisplay {
	if customABIs == nil {
		customABIs = map[string]*abi.ABI{}
	}

	txinfo, err := reader.TxInfoFromHash(tx)
	if err != nil {
		u.Error("getting tx info failed: %s", err)
		return nil
	}

	if txinfo.Tx.To() == nil {
		return nil
	}
	contractAddress := txinfo.Tx.To().Hex()

	isContract, err := IsContract(contractAddress, network)
	if err != nil {
		u.Error("checking tx type failed: %s", err)
		return nil
	}

	var result *jarviscommon.TxResult

	if isContract {
		if a == nil {
			a, err = ConfigToABI(contractAddress, forceERC20ABI, customABI, network)
			if err != nil {
				u.Error("Couldn't get abi for %s: %s", contractAddress, err)
				return nil
			}
		}
		customABIs[strings.ToLower(txinfo.Tx.To().Hex())] = a
		result = analyzer.AnalyzeOffline(&txinfo, GetABI, customABIs, true)
	} else {
		result = analyzer.AnalyzeOffline(&txinfo, GetABI, nil, false)
	}

	return DisplayTxResult(u, result, network, degenMode, tx)
}

func EthTxMonitor(network networks.Network) (*monitor.TxMonitor, error) {
	r, err := EthReader(network)
	if err != nil {
		return nil, err
	}
	return monitor.NewGenericTxMonitor(r), nil
}


func EthBroadcaster(network networks.Network) (*broadcaster.Broadcaster, error) {
	nodes, err := GetNodes(network)
	if err != nil {
		return nil, err
	}
	return broadcaster.NewGenericBroadcaster(nodes), nil
}

func EthReader(network networks.Network) (*reader.EthReader, error) {
	var result *reader.EthReader
	var err error
	nodes, err := GetNodes(network)
	if err != nil {
		return nil, err
	}

	result = reader.NewEthReaderGeneric(nodes, network)
	// etherscanAPIKey := strings.Trim(os.Getenv(ETHERSCAN_API_KEY_VAR), " ")
	// if etherscanAPIKey != "" {
	// 	result.SetEtherscanAPIKey(etherscanAPIKey)
	// }

	// bscscanAPIKey := strings.Trim(os.Getenv(BSCSCAN_API_KEY_VAR), " ")
	// if bscscanAPIKey != "" {
	// 	result.SetBSCScanAPIKey(bscscanAPIKey)
	// }
	return result, nil
}

func isRealAddress(value string) bool {
	valueBig, isHex := big.NewInt(0).SetString(value, 0)
	if !isHex {
		return false
	}
	maxAddrBig, _ := hexutil.DecodeBig(MAX_ADDR)
	minAddrBig, _ := big.NewInt(0).SetString(MIN_ADDR, 0)
	if valueBig.Cmp(maxAddrBig) > 0 || valueBig.Cmp(minAddrBig) <= 0 {
		return false
	}
	return true
}

func GetJarvisValue(value string, network networks.Network) jarviscommon.Value {
	valueBig, ok := big.NewInt(0).SetString(value, 0)
	if !ok {
		// Not a valid integer — treat as a raw string literal.
		return jarviscommon.Value{Raw: value, Kind: jarviscommon.DisplayRaw}
	}

	if !isRealAddress(value) {
		// It's a number but outside the address range.
		// 0x-prefixed literals (hex) display raw; plain decimal numbers
		// get the readable-number separator treatment.
		if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
			return jarviscommon.Value{Raw: value, Kind: jarviscommon.DisplayRaw}
		}
		return jarviscommon.Value{Raw: value, Kind: jarviscommon.DisplayInteger}
	}

	addr := GetJarvisAddress(common.BigToAddress(valueBig).Hex(), network)
	return jarviscommon.Value{Raw: addr.Address, Kind: jarviscommon.DisplayAddress, Address: &addr}
}

// GetJarvisAddress resolves addr using the default (production) address
// resolver. Call sites that already have a resolver (e.g. txanalyzer via
// AnalysisContext) should use that resolver directly so the implementation
// can be swapped in tests.
func GetJarvisAddress(addr string, network networks.Network) jarviscommon.Address {
	return NewEnrichedResolver(network).Resolve(addr)
}

// EnrichedResolver wraps addrbook.Default with a lazy fallback that
// fetches verified contract names from the network's block explorer
// (following proxies) the first time an unknown address is seen.
// Successful lookups land in the persistent jarvis cache so subsequent
// resolves — whether through GetJarvisAddress, the TxAnalyzer, or
// PromptTxConfirmation — return the enriched name with no extra work
// from the caller.
//
// Callers therefore never need to remember to call PrefetchContractName
// manually; any address that flows through the analyzer or util
// helpers gets the same treatment, including addresses decoded out of
// nested calldata.
type EnrichedResolver struct {
	inner   addrbook.AddressResolver
	network networks.Network
}

// NewEnrichedResolver returns an EnrichedResolver backed by addrbook.Default.
func NewEnrichedResolver(network networks.Network) *EnrichedResolver {
	return &EnrichedResolver{
		inner:   addrbook.NewDefault(network),
		network: network,
	}
}

// Resolve first consults the local address book / ERC20 cache. If that
// comes back as "unknown", it best-effort prefetches a verified
// contract name from the explorer and retries the lookup. Failures are
// silent: network errors, rate limits, or unverified contracts all
// just fall back to the original "unknown" result, and the in-memory
// probed-set guarantees we don't retry within the same process.
func (r *EnrichedResolver) Resolve(addr string) jarviscommon.Address {
	a := r.inner.Resolve(addr)
	if a.Desc != "unknown" {
		return a
	}
	PrefetchContractName(addr, r.network)
	return r.inner.Resolve(addr)
}

// PrefetchContractName warms the on-disk address cache with the contract
// display name reported by the network's block explorer for addr. It follows
// proxy contracts to their underlying implementation and renders the
// resulting label as either "<Name>" or "<ProxyName> -> <ImplName>" so the
// next call to addrbook.Default.Resolve (and therefore the next
// GetJarvisAddress) can show a meaningful description for contracts that
// aren't in the local jarvis address book.
//
// Network errors and unverified-source responses are non-fatal — the function
// silently leaves the cache untouched in those cases so callers can use it as
// a "best-effort enrichment" hook without worrying about latency or failure.
func PrefetchContractName(addr string, network networks.Network) {
	if addr == "" {
		return
	}
	addrLower := strings.ToLower(addr)
	cacheKey := fmt.Sprintf("%s_contract_name", addrLower)
	if existing, found := cache.GetCache(cacheKey); found && existing != "" {
		return
	}
	// In-memory "already tried" guard: keeps us from hammering the
	// block explorer for unverified / unknown contracts on repeated
	// lookups within a single jarvis process. Positive results land
	// in the persistent disk cache above, so they short-circuit this
	// check on the next run; negative results are deliberately not
	// persisted (a contract might get verified later).
	if markedProbed(network, addrLower) {
		return
	}
	markProbed(network, addrLower)

	r, err := EthReader(network)
	if err != nil {
		return
	}
	info, err := r.GetContractInfo(addr)
	if err != nil || !info.IsVerified || info.Name == "" {
		return
	}
	label := info.Name

	if info.IsProxy && info.Implementation != "" {
		implInfo, err := r.GetContractInfo(info.Implementation)
		if err == nil && implInfo.IsVerified && implInfo.Name != "" && implInfo.Name != info.Name {
			label = fmt.Sprintf("%s -> %s", info.Name, implInfo.Name)
		}
		_ = cache.SetCache(
			fmt.Sprintf("%s_contract_name", strings.ToLower(info.Implementation)),
			implInfo.Name,
		)
	}
	_ = cache.SetCache(cacheKey, label)
}

// contractNameProbed tracks (network, address) pairs whose explorer
// contract-name lookup has already been attempted this process. Used
// only by PrefetchContractName; the shared jarvis on-disk cache
// handles cross-run persistence of *successful* lookups.
var (
	contractNameProbedMu sync.Mutex
	contractNameProbed   = map[string]struct{}{}
)

func probeKey(network networks.Network, addrLower string) string {
	return network.GetName() + "|" + addrLower
}

func markedProbed(network networks.Network, addrLower string) bool {
	contractNameProbedMu.Lock()
	defer contractNameProbedMu.Unlock()
	_, ok := contractNameProbed[probeKey(network, addrLower)]
	return ok
}

func markProbed(network networks.Network, addrLower string) {
	contractNameProbedMu.Lock()
	defer contractNameProbedMu.Unlock()
	contractNameProbed[probeKey(network, addrLower)] = struct{}{}
}

func isHttpURL(path string) bool {
	u, err := url.ParseRequestURI(path)
	if err != nil {
		return false
	}
	if u.Scheme == "" {
		return false
	}
	return true
}

func ReadCustomABIString(
	addr string,
	pathOrAddress string,
	network networks.Network,
) (str string, err error) {
	if isRealAddress(pathOrAddress) {
		return GetABIString(pathOrAddress, network)
	} else if isHttpURL(pathOrAddress) {
		str, err = GetABIStringFromURL(pathOrAddress)
	} else if str, err = GetABIStringFromFile(pathOrAddress); err != nil {
		str = pathOrAddress
		err = nil
	}

	return str, err
}

type coingeckopriceresponse map[string]map[string]float64

func GetETHPriceInUSD() (float64, error) {
	resp, err := http.Get(
		"https://api.coingecko.com/api/v3/simple/price?ids=ethereum&vs_currencies=usd&include_market_cap=false&include_24hr_vol=false&include_24hr_change=false&include_last_updated_at=false",
	)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	priceres := coingeckopriceresponse{}
	err = json.Unmarshal(body, &priceres)
	if err != nil {
		return 0, err
	}
	return priceres["ethereum"]["usd"], nil
}

func GetCoinGeckoRateInUSD(token string) (float64, error) {
	if strings.ToLower(token) == "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" {
		return GetETHPriceInUSD()
	}
	resp, err := http.Get(
		fmt.Sprintf(
			"https://api.coingecko.com/api/v3/simple/token_price/ethereum?contract_addresses=%s&vs_currencies=USD&include_market_cap=false&include_24hr_vol=false&include_24hr_change=false&include_last_updated_at=false",
			token,
		),
	)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	priceres := coingeckopriceresponse{}
	err = json.Unmarshal(body, &priceres)
	if err != nil {
		return 0, err
	}
	return priceres[strings.ToLower(token)]["usd"], nil
}

func ReadCustomABI(addr string, pathOrAddress string, network networks.Network) (a *abi.ABI, err error) {
	str, err := ReadCustomABIString(addr, pathOrAddress, network)
	if err != nil {
		return nil, err
	}

	a, err = GetABIFromString(str)
	if err != nil {
		return a, err
	}

	cacheKey := fmt.Sprintf("%s_abi", addr)
	cache.SetCache(cacheKey, str)
	return a, nil
}

func GetABIStringFromFile(filepath string) (string, error) {
	abiBytes, err := os.ReadFile(filepath)
	return string(abiBytes), err
}

func GetABIStringFromURL(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

func GetABIFromBytes(abiBytes []byte) (*abi.ABI, error) {
	result, err := abi.JSON(bytes.NewReader(abiBytes))
	return &result, err
}

func GetABIFromString(abiStr string) (*abi.ABI, error) {
	result, err := abi.JSON(strings.NewReader(abiStr))
	return &result, err
}

func GetABIStringBypassCache(addr string, network networks.Network) (string, error) {
	cacheKey := fmt.Sprintf("%s_abi", addr)
	// not found from cache, getting from etherscan or equivalent websites
	reader, err := EthReader(network)
	if err != nil {
		return "", err
	}
	abiStr, err := reader.GetABIString(addr)
	if err != nil {
		return "", err
	}

	cache.SetCache(
		cacheKey,
		abiStr,
	)
	return abiStr, nil
}

func IsContract(addr string, network networks.Network) (bool, error) {
	cacheKey := fmt.Sprintf("%s_%s_is_contract", strings.ToLower(addr), network)
	_, found := cache.GetCache(cacheKey)
	if found {
		return true, nil
	}

	reader, err := EthReader(network)
	if err != nil {
		return false, err
	}

	code, err := reader.GetCode(addr)
	if err != nil {
		return false, err
	}

	isContract := len(code) > 0

	if isContract {
		cache.SetCache(
			cacheKey,
			"true",
		)
	}
	return isContract, nil
}

func GetABIString(addr string, network networks.Network) (string, error) {
	cacheKey := fmt.Sprintf("%s_abi", strings.ToLower(addr))
	cached, found := cache.GetCache(cacheKey)
	if found {
		return cached, nil
	}
	return GetABIStringBypassCache(addr, network)
}

func ConfigToABI(
	address string,
	forceERC20ABI bool,
	customABI string,
	network networks.Network,
) (*abi.ABI, error) {
	if forceERC20ABI {
		return jarviscommon.GetERC20ABI(), nil
	}
	if customABI != "" {
		return ReadCustomABI(address, customABI, network)
	}
	a, err := GetABI(address, network)
	if err != nil {
		return a, err
	}

	if IsProxyABI(a) {
		r, err := EthReader(network)
		if err != nil {
			return nil, err
		}

		impl, err := r.ImplementationOf(-1, address)
		if err != nil {
			fmt.Printf("getting implementation of %s failed: %s\n", address, err)
			return nil, err
		}
		a, err := GetABI(impl.Hex(), network)
		if err != nil {
			fmt.Printf("getting abi for implementation %s of %s failed: %s\n", impl.Hex(), address, err)
		}
		return a, err
	}

	return a, nil
}

func GetGnosisMsigDeployByteCode(ctorBytes []byte) ([]byte, error) {
	bytecode, err := hexutil.Decode(gnosisMsigDeployCode)
	if err != nil {
		return []byte{}, err
	}
	data := append(bytecode, ctorBytes...)
	return data, nil
}

func GetGnosisMsigABI() *abi.ABI {
	result, err := abi.JSON(strings.NewReader(gnosisMsigABI))
	if err != nil {
		panic(err)
	}
	return &result
}

func GetABI(addr string, network networks.Network) (*abi.ABI, error) {
	abiStr, err := GetABIString(addr, network)
	if err != nil {
		return nil, err
	}

	result, err := GetABIFromString(abiStr)
	if err == nil {
		return result, nil
	}

	// now abiStr is an invalid abi string
	// try bypassing the cache and query again
	abiStr, err = GetABIStringBypassCache(addr, network)
	if err != nil {
		return nil, err
	}
	return GetABIFromString(abiStr)
}

func IsProxyABI(a *abi.ABI) bool {
	isGnosis, _ := IsGnosisMultisig(a)
	if isGnosis {
		return false
	}

	// if a.Fallback.String() != "" {
	// 	return true
	// }

	for _, m := range PROXY_METHODS {
		_, found := a.Methods[m]
		if !found {
			return false
		}
	}
	return true
}

func IsERC20ABI(a *abi.ABI) bool {
	for _, m := range ERC20_METHODS {
		_, found := a.Methods[m]
		if !found {
			return false
		}
	}
	return true
}

func IsGnosisMultisig(a *abi.ABI) (bool, error) {
	methods := []string{
		"confirmations",
		"getTransactionCount",
		"isConfirmed",
		"getConfirmationCount",
		"getOwners",
		"transactions",
		"transactionCount",
		"required",
	}

	for _, m := range methods {
		_, found := a.Methods[m]
		if !found {
			return false, nil
		}
	}
	return true, nil
}

func GetBalances(
	wallets []string,
	tokens []string,
	network networks.Network,
) (balances map[common.Address][]*big.Int, block int64, err error) {
	return GetHistoryBalances(-1, wallets, tokens, network)
}

func GetHistoryBalances(
	atBlock int64,
	wallets []string,
	tokens []string,
	network networks.Network,
) (balances map[common.Address][]*big.Int, block int64, err error) {
	helperABI := jarviscommon.GetMultiCallABI()
	erc20ABI := jarviscommon.GetERC20ABI()

	mc, err := NewMultiCall(network)
	if err != nil {
		return nil, 0, err
	}

	balances = map[common.Address][]*big.Int{}

	for _, wallet := range wallets {
		wAddr := jarviscommon.HexToAddress(wallet)
		for i, token := range tokens {
			index := i
			oneResult := big.NewInt(0)
			balances[wAddr] = append(balances[wAddr], oneResult)
			if strings.ToLower(token) == "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" {
				mc.RegisterWithHook(
					&oneResult,
					func(r interface{}) error {
						balances[wAddr][index] = *r.(**big.Int)
						return nil
					},
					network.MultiCallContract(),
					helperABI,
					"getEthBalance",
					jarviscommon.HexToAddress(wallet),
				)
			} else {
				mc.RegisterWithHook(
					&oneResult,
					func(r interface{}) error {
						balances[wAddr][index] = *r.(**big.Int)
						return nil
					},
					token,
					erc20ABI,
					"balanceOf",
					jarviscommon.HexToAddress(wallet),
				)
			}
		}
	}

	block, err = mc.Do(atBlock)

	return balances, block, err
}

func NewMultiCall(network networks.Network) (*reader.MultipleCall, error) {
	r, err := EthReader(network)
	if err != nil {
		return nil, err
	}
	return reader.NewMultiCall(r, network.MultiCallContract()), nil
}

