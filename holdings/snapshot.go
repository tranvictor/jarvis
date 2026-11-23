package holdings

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// Snapshot is the JSON shape of a book. Amounts and prices stay strings.
// Percents are signed numbers. Glyphs are a terminal concern.
type Snapshot struct {
	Address           string        `json:"address"`
	Name              string        `json:"name,omitempty"`
	TotalUSD          float64       `json:"totalUsd"`
	Change24hUSD      *float64      `json:"change24hUsd,omitempty"`
	Change24hPercent  *float64      `json:"change24hPercent,omitempty"`
	MinUSD            float64       `json:"minUsd"`
	HiddenBelowMin    int           `json:"hiddenBelowMin"`
	Unpriced          int           `json:"unpriced"`
	Warnings          []string      `json:"warnings"`
	Positions         []snapshotPos `json:"positions"`
	FetchedAt         string        `json:"fetchedAt"`
	KyberAmountChains []string      `json:"kyberAmountChains,omitempty"`
}

type snapshotPos struct {
	ChainID          int      `json:"chainId"`
	Chain            string   `json:"chain"`
	TokenAddress     string   `json:"tokenAddress"`
	Symbol           string   `json:"symbol,omitempty"`
	Amount           string   `json:"amount"`
	RawAmount        string   `json:"rawAmount,omitempty"`
	AmountSource     string   `json:"amountSource"`
	ValueUSD         *float64 `json:"valueUsd"`
	Mark             string   `json:"mark,omitempty"`
	Price            string   `json:"price,omitempty"`
	PriceAtMs        int64    `json:"priceAtMs,omitempty"`
	Change24hPercent *float64 `json:"change24hPercent,omitempty"`
	Change1hPercent  *float64 `json:"change1hPercent,omitempty"`
	Buy              string   `json:"buy,omitempty"`
	Sell             string   `json:"sell,omitempty"`
	SpreadBps        *float64 `json:"spreadBps,omitempty"`
	RiskLevel        string   `json:"riskLevel,omitempty"`
	RiskReasons      []string `json:"riskReasons,omitempty"`
	BuyTaxBps        *int     `json:"buyTaxBps,omitempty"`
	SellTaxBps       *int     `json:"sellTaxBps,omitempty"`
	FeeOnTransfer    *bool    `json:"feeOnTransfer,omitempty"`
	Rebasing         *bool    `json:"rebasing,omitempty"`
	ExitTier         string   `json:"exitTier,omitempty"`
}

// Snapshot copies the book into the JSON document.
func (b *Book) Snapshot() Snapshot {
	name := b.Name
	if strings.EqualFold(name, "unknown") {
		name = ""
	}
	s := Snapshot{
		Address:           b.Address,
		Name:              name,
		TotalUSD:          b.Total,
		Change24hUSD:      b.ChangeUSD,
		Change24hPercent:  b.ChangePct,
		MinUSD:            b.MinUSD,
		HiddenBelowMin:    b.Hidden,
		Unpriced:          b.Unpriced,
		Warnings:          b.warningLines(),
		KyberAmountChains: append([]string(nil), b.KyberChains...),
		FetchedAt:         b.FetchedAt.UTC().Format(time.RFC3339),
	}
	if s.Warnings == nil {
		s.Warnings = []string{}
	}
	for _, p := range b.Positions {
		sp := snapshotPos{
			ChainID:          p.ChainID,
			Chain:            p.ChainName,
			TokenAddress:     p.TokenAddress,
			Symbol:           p.Symbol,
			Amount:           p.Amount,
			RawAmount:        p.RawAmount,
			AmountSource:     p.AmountSource,
			ValueUSD:         p.Value,
			Mark:             p.Mark,
			Price:            p.Price,
			PriceAtMs:        p.PriceAtMs,
			Change24hPercent: p.Change24h,
			Change1hPercent:  p.Change1h,
			Buy:              p.BuyUSD,
			Sell:             p.SellUSD,
			RiskLevel:        p.RiskLevel,
			RiskReasons:      p.RiskReasons,
			BuyTaxBps:        p.BuyTaxBps,
			SellTaxBps:       p.SellTaxBps,
			FeeOnTransfer:    p.IsFOT,
			Rebasing:         p.IsRebasing,
			ExitTier:         p.ExitTier,
		}
		if bps, ok := p.spreadBps(); ok {
			sp.SpreadBps = &bps
		}
		s.Positions = append(s.Positions, sp)
	}
	if s.Positions == nil {
		s.Positions = []snapshotPos{}
	}
	return s
}

// WriteJSON writes Snapshot to path.
func WriteJSON(path string, b *Book) error {
	data, err := json.MarshalIndent(b.Snapshot(), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
