package holdings

import (
	"context"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tranvictor/jarvis/kyber"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util/reader"
)

const nativeSentinel = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

// Loader fetches a book. It keeps chain metadata and RPC readers between
// the snapshot and each live refresh.
type Loader struct {
	Client  *kyber.Client
	MinUSD  float64
	ExitMin float64
	Chains  map[int]struct{}

	chainName    map[int]string
	native       map[int]string
	nativeSymbol map[int]string
	chainsLoaded bool
	chainErr     error

	readers  map[int]*reader.EthReader
	noReader map[int]bool
	mu       sync.Mutex
	lastPipe map[int]pipeline
}

// NewLoader builds a loader. ExitMin is the USD size that earns an exit tier.
func NewLoader(c *kyber.Client, minUSD, exitMin float64, chains map[int]struct{}) *Loader {
	if exitMin < 0 {
		exitMin = 0
	}
	if minUSD < 0 {
		minUSD = 0
	}
	return &Loader{
		Client:       c,
		MinUSD:       minUSD,
		ExitMin:      exitMin,
		Chains:       chains,
		chainName:    map[int]string{},
		native:       map[int]string{},
		nativeSymbol: map[int]string{},
		readers:      map[int]*reader.EthReader{},
		noReader:     map[int]bool{},
	}
}

// Load reads balances, marks, chain amounts and exit tiers.
func (l *Loader) Load(ctx context.Context, wallet string) (*Book, error) {
	b := &Book{
		Address:  wallet,
		MinUSD:   l.MinUSD,
		Pipeline: map[int]pipeline{},
		Degraded: map[int]map[string]bool{},
	}
	if err := l.fill(ctx, b, nil); err != nil {
		return nil, err
	}
	return b, nil
}

// Refresh replaces holdings and marks, keeping a newer socket price when
// the REST snapshot is older.
func (l *Loader) Refresh(ctx context.Context, b *Book) error {
	prev := map[string]*Position{}
	for _, p := range b.Positions {
		prev[p.Key()] = p
	}
	return l.fill(ctx, b, prev)
}

// RefreshMarket re-reads prices after a pipeline is marked degraded.
func (l *Loader) RefreshMarket(ctx context.Context, b *Book) error {
	markets, err := l.Client.Market(ctx, addressesOf(b.Positions), []string{"1h", "24h"})
	if err != nil && len(markets) == 0 {
		b.MarketNote = "market prices unavailable · values use the catalog price"
		return err
	}
	if err != nil {
		b.MarketNote = "market prices incomplete · some values use the catalog price"
	} else {
		b.MarketNote = ""
	}
	l.applyMarkets(b.Positions, markets)
	if l.lastPipe != nil {
		b.Pipeline = l.lastPipe
	}
	for _, p := range b.Positions {
		p.revalue()
	}
	b.recomputeTotals()
	b.StatsAt = time.Now()
	return nil
}

func (l *Loader) fill(ctx context.Context, b *Book, prev map[string]*Position) error {
	l.ensureChains(ctx)
	var chainID *int
	if len(l.Chains) == 1 {
		for id := range l.Chains {
			id := id
			chainID = &id
		}
	}
	bals, err := l.Client.Balances(ctx, b.Address, chainID)
	if err != nil {
		return err
	}
	positions := l.positionsFrom(bals, prev)
	byChain := addressesOf(positions)

	var tokErr, mktErr error
	var tokens map[int][]kyber.Token
	var markets map[int]kyber.ChainMarket
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		tokens, tokErr = l.Client.Tokens(ctx, byChain)
	}()
	go func() {
		defer wg.Done()
		markets, mktErr = l.Client.Market(ctx, byChain, []string{"1h", "24h"})
	}()
	go func() {
		defer wg.Done()
		l.readAmounts(b.Address, positions)
	}()
	wg.Wait()

	if tokErr != nil {
		b.SecurityNote = "token security unavailable"
	} else {
		b.SecurityNote = ""
	}
	applyTokens(positions, tokens)
	if mktErr != nil && len(markets) == 0 {
		b.MarketNote = "market prices unavailable · values use the catalog price"
	} else if mktErr != nil {
		b.MarketNote = "market prices incomplete · some values use the catalog price"
	} else {
		b.MarketNote = ""
	}
	l.applyMarkets(positions, markets)
	if l.lastPipe != nil {
		b.Pipeline = l.lastPipe
	}
	for _, p := range positions {
		p.revalue()
	}
	l.exits(ctx, positions)

	b.Positions = positions
	b.recomputeTotals()
	b.sortPositions()
	now := time.Now()
	b.FetchedAt = now
	b.BalancesAt = now
	if mktErr == nil {
		b.StatsAt = now
	}
	if b.Pipeline == nil {
		b.Pipeline = map[int]pipeline{}
	}
	if b.Degraded == nil {
		b.Degraded = map[int]map[string]bool{}
	}
	return nil
}

func (l *Loader) ensureChains(ctx context.Context) {
	if l.chainsLoaded {
		return
	}
	l.chainsLoaded = true
	chains, err := l.Client.Chains(ctx)
	if err != nil {
		l.chainErr = err
		return
	}
	for _, c := range chains {
		if c.Name != "" {
			l.chainName[c.ChainID] = c.Name
		}
		if c.NativeToken.Address != "" {
			l.native[c.ChainID] = strings.ToLower(c.NativeToken.Address)
		}
		if c.NativeToken.Symbol != nil && *c.NativeToken.Symbol != "" {
			l.nativeSymbol[c.ChainID] = *c.NativeToken.Symbol
		}
	}
}

func (l *Loader) nameOf(id int) string {
	if n := l.chainName[id]; n != "" {
		return n
	}
	if n, err := networks.GetNetworkByID(uint64(id)); err == nil {
		return n.GetName()
	}
	return "chain " + strconv.Itoa(id)
}

func (l *Loader) quoteOf(id int) string {
	if s := l.nativeSymbol[id]; s != "" {
		return s
	}
	if n, err := networks.GetNetworkByID(uint64(id)); err == nil {
		return n.GetNativeTokenSymbol()
	}
	return ""
}

func (l *Loader) positionsFrom(bals []kyber.Balance, prev map[string]*Position) []*Position {
	seen := map[string]bool{}
	var out []*Position
	for _, bal := range bals {
		if len(l.Chains) > 0 {
			if _, ok := l.Chains[bal.ChainID]; !ok {
				continue
			}
		}
		p := l.position(bal)
		if p.TokenAddress == "" || seen[p.Key()] {
			continue
		}
		if zeroAmount(p.Amount) && zeroAmount(p.RawAmount) {
			continue
		}
		if old := prev[p.Key()]; old != nil {
			copyPrices(p, old)
		}
		seen[p.Key()] = true
		out = append(out, p)
	}
	return out
}

func (l *Loader) position(b kyber.Balance) *Position {
	addr := strings.ToLower(strings.TrimSpace(b.TokenAddress))
	native := l.native[b.ChainID]
	if native == "" {
		native = nativeSentinel
	}
	p := &Position{
		ChainID:      b.ChainID,
		ChainName:    l.nameOf(b.ChainID),
		TokenAddress: addr,
		Symbol:       b.Symbol,
		Decimals:     b.Decimals,
		Amount:       strings.TrimSpace(b.Amount),
		RawAmount:    strings.TrimSpace(b.RawAmount),
		AmountSource: amountKyber,
		QuoteSymbol:  l.quoteOf(b.ChainID),
		Native:       addr == native || addr == nativeSentinel,
	}
	if p.Amount == "" && p.RawAmount != "" {
		if raw, ok := new(big.Int).SetString(p.RawAmount, 10); ok {
			p.Amount = humanAmount(raw, p.Decimals)
		}
	}
	if b.PriceUSD.V != nil && *b.PriceUSD.V != 0 {
		p.Indicative = strconv.FormatFloat(*b.PriceUSD.V, 'f', -1, 64)
	}
	return p
}

func copyPrices(dst, src *Position) {
	dst.SellUSD, dst.SellAtMs = src.SellUSD, src.SellAtMs
	dst.BuyUSD, dst.BuyAtMs = src.BuyUSD, src.BuyAtMs
	dst.OnchainE = src.OnchainE
	dst.SettlementUSD = src.SettlementUSD
	dst.SettlementAtMs = src.SettlementAtMs
	dst.SettlementEpoch = src.SettlementEpoch
	dst.SettlementBlock = src.SettlementBlock
	dst.HasSettlement = src.HasSettlement
	dst.Change24h = src.Change24h
	dst.Change1h = src.Change1h
	dst.RiskLevel = src.RiskLevel
	dst.RiskReasons = src.RiskReasons
	dst.SellTaxBps = src.SellTaxBps
	dst.BuyTaxBps = src.BuyTaxBps
	dst.IsFOT = src.IsFOT
	dst.IsRebasing = src.IsRebasing
	dst.IsHoneypot = src.IsHoneypot
	dst.ExitTier = src.ExitTier
	dst.ExitQuoted = src.ExitQuoted
	dst.ExitScore = src.ExitScore
	dst.ExitVenues = src.ExitVenues
	dst.ExitMethod = src.ExitMethod
	dst.ExitURL = src.ExitURL
	dst.ExitRaw = src.ExitRaw
}

func applyTokens(ps []*Position, tokens map[int][]kyber.Token) {
	by := map[string]*Position{}
	for _, p := range ps {
		by[p.Key()] = p
	}
	for chain, list := range tokens {
		for _, t := range list {
			p := by[strconv.Itoa(chain)+":"+strings.ToLower(t.TokenAddress)]
			if p == nil {
				continue
			}
			if t.Symbol != nil && *t.Symbol != "" && p.Symbol == "" {
				p.Symbol = *t.Symbol
			}
			if t.Decimals != nil && p.Decimals == 0 {
				p.Decimals = *t.Decimals
			}
			if t.Category != nil {
				p.IsRebasing = t.Category.IsRebasing
			}
			if t.Security == nil {
				continue
			}
			if t.Security.RiskLevel != nil {
				p.RiskLevel = *t.Security.RiskLevel
			}
			p.RiskReasons = t.Security.RiskReasons
			p.IsHoneypot = t.Security.IsHoneypot
			p.IsFOT = t.Security.IsFOT
			if t.Security.Tax != nil {
				p.BuyTaxBps = t.Security.Tax.BuyBps
				p.SellTaxBps = t.Security.Tax.SellBps
			}
		}
	}
}

func (l *Loader) applyMarkets(ps []*Position, markets map[int]kyber.ChainMarket) {
	if len(markets) == 0 {
		return
	}
	pipe := map[int]pipeline{}
	for id, m := range markets {
		pipe[id] = pipeline{
			name:       l.nameOf(id),
			onchain:    m.Meta.OnchainDegraded,
			settlement: m.Meta.SettlementDegraded,
			stats:      m.Meta.StatsDegraded,
		}
		for _, p := range ps {
			if p.ChainID != id {
				continue
			}
			tm, ok := m.Tokens[strings.ToLower(p.TokenAddress)]
			if !ok {
				continue
			}
			applyMarket(p, tm)
		}
	}
	l.lastPipe = pipe
}

func applyMarket(p *Position, tm kyber.TokenMarket) {
	if tm.Price != nil && tm.Price.Onchain != nil {
		if s := tm.Price.Onchain.Sell; s != nil && s.USD != "" && (p.SellAtMs == 0 || s.AtMs == 0 || s.AtMs >= p.SellAtMs) {
			p.SellUSD = s.USD
			p.SellAtMs = s.AtMs
			if s.AtMs > p.OnchainE {
				p.OnchainE = s.AtMs
			}
		}
		if s := tm.Price.Onchain.Buy; s != nil && s.USD != "" && (p.BuyAtMs == 0 || s.AtMs == 0 || s.AtMs >= p.BuyAtMs) {
			p.BuyUSD = s.USD
			p.BuyAtMs = s.AtMs
			if s.AtMs > p.OnchainE {
				p.OnchainE = s.AtMs
			}
		}
	}
	if tm.Price != nil && tm.Price.Settlement != nil && tm.Price.Settlement.USD != nil && *tm.Price.Settlement.USD != "" {
		cur := tm.Price.Settlement.Cursor
		newer := !p.HasSettlement || cur.Epoch > p.SettlementEpoch || (cur.Epoch == p.SettlementEpoch && cur.BlockNumber >= p.SettlementBlock)
		if newer {
			p.SettlementUSD = *tm.Price.Settlement.USD
			p.SettlementAtMs = tm.Price.Settlement.AsOfMs
			p.SettlementEpoch = cur.Epoch
			p.SettlementBlock = cur.BlockNumber
			p.HasSettlement = true
		}
	}
	if st := tm.Stats["24h"]; st != nil && st.PriceChangePercent != nil {
		v := *st.PriceChangePercent
		p.Change24h = &v
	}
	if st := tm.Stats["1h"]; st != nil && st.PriceChangePercent != nil {
		v := *st.PriceChangePercent
		p.Change1h = &v
	}
}

func (l *Loader) exits(ctx context.Context, ps []*Position) {
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, p := range ps {
		if p.honeypot() || p.Native || p.Value == nil || *p.Value < l.ExitMin || l.ExitMin == 0 {
			continue
		}
		native := l.native[p.ChainID]
		if native == "" {
			native = nativeSentinel
		}
		if strings.EqualFold(native, p.TokenAddress) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(p *Position, native string) {
			defer wg.Done()
			defer func() { <-sem }()
			liq, ex, err := l.Client.PairLiquidity(ctx, p.ChainID, p.TokenAddress, native)
			if err != nil || liq.Tier == "" {
				return
			}
			p.ExitTier = liq.Tier
			p.ExitQuoted = true
			p.ExitScore = liq.Score
			p.ExitVenues = liq.VenueCount
			p.ExitMethod = ex.Method
			p.ExitURL = ex.URL
			p.ExitRaw = ex.Raw
		}(p, native)
	}
	wg.Wait()
}

func addressesOf(ps []*Position) map[int][]string {
	m := map[int][]string{}
	seen := map[string]bool{}
	for _, p := range ps {
		if seen[p.Key()] {
			continue
		}
		seen[p.Key()] = true
		m[p.ChainID] = append(m[p.ChainID], p.TokenAddress)
	}
	return m
}
