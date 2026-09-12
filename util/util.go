package util

import (
	"bytes"
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

	jarviscommon "github.com/tranvictor/jarvis/common"
	db "github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util/addrbook"
	"github.com/tranvictor/jarvis/util/broadcaster"
	"github.com/tranvictor/jarvis/util/cache"
	"github.com/tranvictor/jarvis/util/ens"
	"github.com/tranvictor/jarvis/util/explorers"
	"github.com/tranvictor/jarvis/util/monitor"
	"github.com/tranvictor/jarvis/util/reader"
)

const (
	ETH_ADDR string = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	MAX_ADDR string = "0xffffffffffffffffffffffffffffffffffffffff"
	MIN_ADDR string = "0x00000000000000ffffffffffffffffffffffffff"
)

// GetExactAddressFromDatabases resolves str against the local address
// database. Kept as a separate name for callers (e.g. `jarvis whois`) that
// historically wanted "exact" lookups; since db.GetAddresses already
// resolves address-shaped input by exact key match (see
// jarviscommon.LooksLikeAddress) and only fuzzy-matches free text, this is
// now equivalent to getRelevantAddressesFromDatabases.
func GetExactAddressFromDatabases(str string) (addrs []string, names []string, scores []int) {
	return getRelevantAddressesFromDatabases(str)
}

func getRelevantAddressesFromDatabases(str string) (addrs []string, names []string, scores []int) {
	addrDescs, matchScores := db.GetAddresses(str)
	for i, addr := range addrDescs {
		addrs = append(addrs, addr.Address)
		names = append(names, addr.Desc)
		scores = append(scores, matchScores[i])
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

// ScanForTxs finds network-prefixed or bare 32-byte transaction hashes.
// nwks[i] is the canonical network for hashes[i]: a prefix when present
// (mainnet:0x… / bsc 0x…), otherwise defaultNetwork, otherwise Ethereum
// mainnet. Aliases such as "ethereum" are canonicalized to GetName().
func ScanForTxs(para, defaultNetwork string) (nwks []string, hashes []string) {
	networkNames := networks.GetSupportedNetworkNames()
	regexStr := strings.Join(networkNames, "|")
	regexStr = fmt.Sprintf(
		"(?i)(?:(?P<network>%s)(?:.{0,}?))?(?P<address>(?:0x)?(?:[0-9a-fA-F]{64}))",
		regexStr,
	)

	re := regexp.MustCompile(regexStr)
	for _, match := range re.FindAllStringSubmatch(para, -1) {
		nwks = append(nwks, resolveTxNetwork(strings.ToLower(match[1]), defaultNetwork))
		hashes = append(hashes, match[2])
	}
	return
}

// resolveTxNetwork turns a captured prefix (possibly empty) into the
// canonical network name jarvis will use for that hash.
func resolveTxNetwork(captured, defaultNetwork string) string {
	name := captured
	if name == "" {
		name = strings.ToLower(strings.TrimSpace(defaultNetwork))
	}
	if name == "" {
		name = networks.EthereumMainnet.GetName()
	}
	if net, err := networks.GetNetwork(name); err == nil {
		return net.GetName()
	}
	return name
}

// ScanForTxHashes is ScanForTxs dropping the network names.
func ScanForTxHashes(para string) []string {
	_, hashes := ScanForTxs(para, "")
	if hashes == nil {
		return []string{}
	}
	return hashes
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
		u.Success("✓ broadcast  %s  %s", network.GetName(), t.Hash().Hex())
	}
}

// WaitForTx blocks until the tx is mined, reverted or lost, showing a live
// status line that tracks what the monitor sees (not yet in mempool → in
// mempool → outcome). It returns the final status.
func WaitForTx(u ui.UI, mo *monitor.TxMonitor, hash string) string {
	return waitForStatuses(u, mo.MakeStatusChannel(hash))
}

// waitForStatuses drives the status line from a monitor status channel.
func waitForStatuses(u ui.UI, statuses <-chan string) string {
	progress := u.Spinner("waiting for the tx to show up in the mempool…")
	for st := range statuses {
		switch st {
		case "pending":
			progress.Update("in mempool, waiting to be mined…")
		case "done":
			progress.Stop(ui.StyledText{
				Text:     fmt.Sprintf("✓ mined after %s", progress.Elapsed().Round(time.Second)),
				Severity: ui.SeveritySuccess,
			})
			return st
		case "reverted":
			progress.Stop(ui.StyledText{
				Text:     fmt.Sprintf("✗ reverted after %s", progress.Elapsed().Round(time.Second)),
				Severity: ui.SeverityError,
			})
			return st
		case "lost":
			progress.Stop(ui.StyledText{
				Text:     fmt.Sprintf("✗ dropped from the mempool after %s", progress.Elapsed().Round(time.Second)),
				Severity: ui.SeverityError,
			})
			return st
		}
	}
	progress.Stop(ui.StyledText{Text: "✗ stopped waiting", Severity: ui.SeverityError})
	return "unknown"
}

// DisplayWaitAnalyze reports the broadcast result and, when it succeeded,
// waits for the tx to be mined and prints the outcome using layout.
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
	layout TxLayout,
) {
	DisplayBroadcastedTx(u, t, broadcasted, err, network)
	if !broadcasted {
		return
	}
	mo, err := EthTxMonitor(network)
	if err != nil {
		u.Error("Couldn't monitor the tx: %s", err)
		return
	}
	hash := t.Hash().Hex()
	if WaitForTx(u, mo, hash) == "lost" {
		u.Info("Check the hash later with: jarvis info %s", hash)
		return
	}
	AnalyzeAndPrint(u, reader, analyzer, hash, network, false, "", a, customABIs, layout)
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
	layout TxLayout,
	after ...func(*TxDisplay, *jarviscommon.TxResult, map[string]*abi.ABI),
) *TxDisplay {
	if customABIs == nil {
		customABIs = map[string]*abi.ABI{}
	}
	defer cache.Flush()

	txinfo, err := reader.TxInfoFromHash(tx)
	if err != nil {
		u.Error("getting tx info failed: %s", err)
		return nil
	}
	if txinfo.Tx == nil {
		u.Error("transaction not found: %s", tx)
		return nil
	}

	// A contract creation has no destination to classify or fetch an ABI
	// for; the analyzer reports it from the receipt.
	if txinfo.Tx.To() == nil {
		WarmAddresses(addressesFromTxInfo(&txinfo), network)
		result := analyzer.AnalyzeOffline(&txinfo, GetABI, customABIs, false)
		return displayTxResultAfter(u, result, network, layout, tx, customABIs, after)
	}
	contractAddress := txinfo.Tx.To().Hex()

	// Explorer name/ABI lookups and eth_getCode are independent. Starting
	// them together hides the getCode RTT behind the first explorer call
	// and means AnalyzeOffline's per-address Resolve/GetABI hits are warm.
	var isContract bool
	var isContractErr error
	jarviscommon.RunParallel(
		func() error {
			WarmAddresses(addressesFromTxInfo(&txinfo), network)
			return nil
		},
		func() error {
			isContract, isContractErr = IsContract(contractAddress, network)
			return nil
		},
	)
	if isContractErr != nil {
		u.Error("checking tx type failed: %s", isContractErr)
		return nil
	}

	lookup := GetABI
	if isContract {
		if a == nil {
			// An unavailable ABI (unverified contract, explorer outage) must
			// not hide the transaction: the analyzer falls back to the ERC-20
			// ABI and reports what it could not decode. Remember the failure
			// so the analyzer doesn't repeat the lookup for the same address.
			a, err = ConfigToABI(contractAddress, forceERC20ABI, customABI, network)
			if err != nil {
				a = nil
				abiErr := err
				lookup = func(addr string, n networks.Network) (*abi.ABI, error) {
					if strings.EqualFold(addr, contractAddress) {
						return nil, abiErr
					}
					return GetABI(addr, n)
				}
			}
		}
		if a != nil {
			customABIs[strings.ToLower(txinfo.Tx.To().Hex())] = a
		}
	}
	result := analyzer.AnalyzeOffline(&txinfo, lookup, customABIs, isContract)

	return displayTxResultAfter(u, result, network, layout, tx, customABIs, after)
}

func displayTxResultAfter(
	u ui.UI,
	result *jarviscommon.TxResult,
	network networks.Network,
	layout TxLayout,
	tx string,
	customABIs map[string]*abi.ABI,
	after []func(*TxDisplay, *jarviscommon.TxResult, map[string]*abi.ABI),
) *TxDisplay {
	return DisplayTxResultWith(u, result, network, layout, tx, func(d *TxDisplay, result *jarviscommon.TxResult) {
		for _, fn := range after {
			fn(d, result, customABIs)
		}
	})
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
	if addr == "" || jarviscommon.IsZeroAddress(addr) {
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
	if err != nil || !info.IsVerified {
		return
	}
	cacheExplorerABI(addrLower, info.ABI)

	label := info.Name
	if info.IsProxy && info.Implementation != "" {
		implInfo, err := r.GetContractInfo(info.Implementation)
		if err == nil && implInfo.IsVerified {
			cacheExplorerABI(strings.ToLower(info.Implementation), implInfo.ABI)
			if implInfo.Name != "" && implInfo.Name != info.Name {
				label = fmt.Sprintf("%s -> %s", info.Name, implInfo.Name)
			} else if implInfo.Name != "" {
				label = implInfo.Name
			}
			if implInfo.Name != "" {
				_ = cache.SetCache(
					fmt.Sprintf("%s_contract_name", strings.ToLower(info.Implementation)),
					implInfo.Name,
				)
			}
			// A methodless proxy ABI is useless for decoding. Prefer the
			// implementation ABI under the proxy's cache key so GetABI
			// after this prefetch does not hit the explorer again.
			if implInfo.ABI != "" && !explorers.ABIJSONHasFunctions(info.ABI) && explorers.ABIJSONHasFunctions(implInfo.ABI) {
				cacheExplorerABI(addrLower, implInfo.ABI)
			}
		}
	}
	if label == "" {
		return
	}
	_ = cache.SetCache(cacheKey, label)
}

func cacheExplorerABI(addrLower, abiStr string) {
	if abiStr == "" {
		return
	}
	_ = cache.SetCache(fmt.Sprintf("%s_abi", addrLower), abiStr)
}

// prefetchConcurrency bounds parallel explorer/RPC warming. Etherscan-style
// APIs rate-limit around a few calls per second; a small pool still overlaps
// RTTs without turning one tx into a burst of retries.
const prefetchConcurrency = 6

// WarmAddresses prefetches explorer names (and ABIs) for addrs so later
// Resolve/GetABI calls hit the in-process cache instead of running
// sequentially during analysis.
func WarmAddresses(addrs []string, network networks.Network) {
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if addr == "" || jarviscommon.IsZeroAddress(addr) {
			continue
		}
		key := strings.ToLower(addr)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, addr)
	}
	if len(unique) == 0 {
		return
	}

	sem := make(chan struct{}, prefetchConcurrency)
	var wg sync.WaitGroup
	for _, addr := range unique {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			PrefetchContractName(addr, network)
		}(addr)
	}
	wg.Wait()
}

func addressesFromTxInfo(txinfo *jarviscommon.TxInfo) []string {
	if txinfo == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var addrs []string
	add := func(addr string) {
		addr = strings.TrimSpace(addr)
		if addr == "" || jarviscommon.IsZeroAddress(addr) {
			return
		}
		key := strings.ToLower(addr)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		addrs = append(addrs, addr)
	}
	if txinfo.Tx != nil {
		if txinfo.Tx.Extra.From != nil {
			add(txinfo.Tx.Extra.From.Hex())
		}
		if to := txinfo.Tx.To(); to != nil {
			add(to.Hex())
		}
	}
	if txinfo.Receipt != nil {
		if txinfo.Receipt.ContractAddress != (common.Address{}) {
			add(txinfo.Receipt.ContractAddress.Hex())
		}
		for _, l := range txinfo.Receipt.Logs {
			if l != nil {
				add(l.Address.Hex())
			}
		}
	}
	return addrs
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

// IsDelegationDesignator reports whether code is an EIP-7702 delegation
// designator: exactly 0xef0100 followed by a 20-byte address.
func IsDelegationDesignator(code []byte) bool {
	return len(code) == 23 && code[0] == 0xef && code[1] == 0x01 && code[2] == 0x00
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

	// An EIP-7702 delegation designator (0xef0100 ‖ address) marks an EOA
	// that delegates to code; for jarvis's purposes — "does a value transfer
	// land in a contract?", "does this need an ABI?" — it is still a wallet.
	isContract := len(code) > 0 && !IsDelegationDesignator(code)

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
	return GetABI(address, network)
}

// isMethodlessABI reports whether a has no callable functions. Gnosis Safe
// proxies (and many other minimal proxies) verify on explorers as just a
// constructor + fallback, so their parsed ABI has an empty Methods map.
func isMethodlessABI(a *abi.ABI) bool {
	return a == nil || len(a.Methods) == 0
}

// explorerImplementation returns the implementation address reported by the
// block explorer when it flags the contract as a proxy. Empty string means
// the explorer did not provide a usable address.
func explorerImplementation(r *reader.EthReader, address string) string {
	info, err := r.GetContractInfo(address)
	if err != nil || !info.IsProxy {
		return ""
	}
	impl := strings.TrimSpace(info.Implementation)
	if impl == "" {
		return ""
	}
	if !strings.HasPrefix(impl, "0x") && !strings.HasPrefix(impl, "0X") {
		impl = "0x" + impl
	}
	if !common.IsHexAddress(impl) || common.HexToAddress(impl) == (common.Address{}) {
		return ""
	}
	return impl
}

// followProxyImplementation resolves the underlying implementation ABI for
// proxy contracts. It follows explorer-reported implementations first (this
// is what Etherscan already knows for SafeProxy / EIP-1967), then on-chain
// storage slots (EIP-1967, ZeppelinOS, Polygon, Safe slot 0).
//
// followed is true only when an implementation ABI was obtained or a
// classic upgradeable-proxy lookup failed hard (preserving historical
// ConfigToABI error behavior). Methodless ABIs that cannot be resolved
// return followed=false so the caller keeps the original ABI.
func followProxyImplementation(
	address string,
	a *abi.ABI,
	network networks.Network,
) (*abi.ABI, bool, error) {
	classicProxy := IsProxyABI(a)
	methodless := isMethodlessABI(a)
	if !classicProxy && !methodless {
		return nil, false, nil
	}

	r, err := EthReader(network)
	if err != nil {
		if classicProxy {
			return nil, true, err
		}
		return nil, false, nil
	}

	if impl := explorerImplementation(r, address); impl != "" && !sameAddress(impl, address) {
		implABI, err := fetchABI(impl, network)
		if err == nil && !isMethodlessABI(implABI) {
			return implABI, true, nil
		}
	}

	impl, err := r.ImplementationOf(-1, address)
	if err != nil {
		if classicProxy {
			fmt.Printf("getting implementation of %s failed: %s\n", address, err)
			return nil, true, err
		}
		return nil, false, nil
	}
	if impl == (common.Address{}) {
		return nil, false, nil
	}

	implABI, err := fetchABI(impl.Hex(), network)
	if err != nil || isMethodlessABI(implABI) {
		if classicProxy {
			fmt.Printf("getting abi for implementation %s of %s failed: %s\n", impl.Hex(), address, err)
			return implABI, true, err
		}
		return nil, false, nil
	}
	return implABI, true, nil
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

// GnosisMsigSubmissionTopic is the Classic Gnosis multisig Submission event topic
// (keccak of Submission(uint256)), taken from the built-in ABI.
func GnosisMsigSubmissionTopic() common.Hash {
	ev, ok := GetGnosisMsigABI().Events["Submission"]
	if !ok {
		panic("gnosis msig ABI missing Submission event")
	}
	return ev.ID
}

// gnosisMsigTxIDTopicIndex maps Classic event IDs to the topic index that
// holds transactionId. Submission/Execution use topics[1]; Confirmation
// and Revocation index the sender first, so the id is topics[2].
func gnosisMsigTxIDTopicIndex() map[common.Hash]int {
	a := GetGnosisMsigABI()
	idx := map[common.Hash]int{}
	if ev, ok := a.Events["Submission"]; ok {
		idx[ev.ID] = 1
	}
	if ev, ok := a.Events["Execution"]; ok {
		idx[ev.ID] = 1
	}
	if ev, ok := a.Events["ExecutionFailure"]; ok {
		idx[ev.ID] = 1
	}
	if ev, ok := a.Events["Confirmation"]; ok {
		idx[ev.ID] = 2
	}
	if ev, ok := a.Events["Revocation"]; ok {
		idx[ev.ID] = 2
	}
	return idx
}

// GnosisMsigTxIDFromLogs returns the transactionId from the first Classic
// Gnosis event on msigAddr that carries one (Submission, Confirmation,
// Revocation, Execution, ExecutionFailure). confirmTransaction txs only
// emit Confirmation, not Submission.
func GnosisMsigTxIDFromLogs(logs []*types.Log, msigAddr string) *big.Int {
	idx := gnosisMsigTxIDTopicIndex()
	for _, l := range logs {
		if len(l.Topics) == 0 {
			continue
		}
		idIdx, ok := idx[l.Topics[0]]
		if !ok || !strings.EqualFold(l.Address.Hex(), msigAddr) {
			continue
		}
		if len(l.Topics) <= idIdx {
			continue
		}
		return l.Topics[idIdx].Big()
	}
	return nil
}

// GnosisMsigTxIDFromCalldata returns the transactionId encoded in a Classic
// confirmTransaction / revokeConfirmation / executeTransaction call, or nil.
func GnosisMsigTxIDFromCalldata(data []byte) *big.Int {
	if len(data) < 4 {
		return nil
	}
	a := GetGnosisMsigABI()
	for _, name := range []string{"confirmTransaction", "revokeConfirmation", "executeTransaction"} {
		method, ok := a.Methods[name]
		if !ok || !bytes.Equal(data[:4], method.ID) {
			continue
		}
		args, err := method.Inputs.Unpack(data[4:])
		if err != nil || len(args) == 0 {
			return nil
		}
		id, ok := args[0].(*big.Int)
		if !ok {
			return nil
		}
		return id
	}
	return nil
}

// IsGnosisMsigCallData reports whether data's selector is a method on the
// built-in Classic Gnosis multisig ABI (confirmTransaction, submitTransaction,
// executeTransaction, …). Used so the analyzer can decode those calls when
// the explorer ABI is missing or is a methodless proxy.
func IsGnosisMsigCallData(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	_, err := GetGnosisMsigABI().MethodById(data[:4])
	return err == nil
}

func GetABI(addr string, network networks.Network) (*abi.ABI, error) {
	a, err := fetchABI(addr, network)
	if err != nil {
		return a, err
	}
	implABI, followed, err := followProxyImplementation(addr, a, network)
	if followed {
		return implABI, err
	}
	return a, nil
}

// fetchABI returns the ABI the explorer published for addr, without
// following a proxy implementation. GetABI wraps this with followProxyImplementation
// so callers decoding calldata see withdraw/deposit on a WETH proxy, not the
// methodless TransparentUpgradeableProxy ABI.
func fetchABI(addr string, network networks.Network) (*abi.ABI, error) {
	abiStr, err := GetABIString(addr, network)
	if err != nil {
		return nil, err
	}

	result, err := GetABIFromString(abiStr)
	if err == nil {
		return result, nil
	}

	abiStr, err = GetABIStringBypassCache(addr, network)
	if err != nil {
		return nil, err
	}
	return GetABIFromString(abiStr)
}

func sameAddress(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func IsProxyABI(a *abi.ABI) bool {
	isGnosis, _ := IsGnosisMultisig(a)
	if isGnosis {
		return false
	}

	for _, m := range PROXY_METHODS {
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
