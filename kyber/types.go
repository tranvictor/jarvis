package kyber

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// FlexFloat decodes a JSON number or a numeric string. The balances
// payload publishes prices as numbers; other payloads use decimal strings.
type FlexFloat struct {
	V *float64
}

func (f *FlexFloat) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		f.V = nil
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			f.V = nil
			return nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		f.V = &v
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	f.V = &v
	return nil
}

// Balance is one holding from GET /wallets/{address}/balances.
type Balance struct {
	ChainID       int       `json:"chainId"`
	WalletAddress string    `json:"walletAddress"`
	TokenAddress  string    `json:"tokenAddress"`
	RawAmount     string    `json:"rawAmount"`
	Symbol        string    `json:"symbol"`
	Decimals      int       `json:"decimals"`
	Amount        string    `json:"amount"`
	PriceUSD      FlexFloat `json:"priceUsd"`
	ValueUSD      FlexFloat `json:"valueUsd"`
}

// Chain is one entry from GET /chains.
type Chain struct {
	ChainID     int    `json:"chainId"`
	Name        string `json:"name"`
	IsActive    bool   `json:"isActive"`
	NativeToken struct {
		Address  string  `json:"address"`
		Symbol   *string `json:"symbol"`
		Decimals *int    `json:"decimals"`
	} `json:"nativeToken"`
}

// Token is the subset of the catalog token used to describe a holding.
type Token struct {
	ChainID      int            `json:"chainId"`
	TokenAddress string         `json:"tokenAddress"`
	Symbol       *string        `json:"symbol"`
	Decimals     *int           `json:"decimals"`
	Security     *TokenSecurity `json:"security"`
	Category     *struct {
		Primary    string `json:"primary"`
		IsRebasing *bool  `json:"isRebasing"`
	} `json:"category"`
}

// TokenSecurity is the swap-safety block. Null fields mean the check
// has not been measured.
type TokenSecurity struct {
	RiskLevel   *string  `json:"riskLevel"`
	RiskReasons []string `json:"riskReasons"`
	IsHoneypot  *bool    `json:"isHoneypot"`
	IsFOT       *bool    `json:"isFOT"`
	Tax         *struct {
		BuyBps  *int `json:"buyBps"`
		SellBps *int `json:"sellBps"`
	} `json:"tax"`
}

// ChainMarket is the market snapshot for one chain.
type ChainMarket struct {
	Meta   ChainMarketMeta        `json:"meta"`
	Tokens map[string]TokenMarket `json:"tokens"`
}

// ChainMarketMeta reports which price pipelines are late.
type ChainMarketMeta struct {
	OnchainDegraded    bool `json:"onchainDegraded"`
	SettlementDegraded bool `json:"settlementDegraded"`
	StatsDegraded      bool `json:"statsDegraded"`
}

// TokenMarket is buy, sell, settlement and window stats for one token.
type TokenMarket struct {
	Price *struct {
		Onchain *struct {
			Buy  *PriceSide `json:"buy"`
			Sell *PriceSide `json:"sell"`
		} `json:"onchain"`
		Settlement *struct {
			USD    *string `json:"usd"`
			AsOfMs int64   `json:"asOfMs"`
			Cursor struct {
				Epoch       int64 `json:"epoch"`
				BlockNumber int64 `json:"blockNumber"`
			} `json:"cursor"`
		} `json:"settlement"`
	} `json:"price"`
	Stats map[string]*struct {
		PriceChangePercent *float64 `json:"priceChangePercent"`
	} `json:"stats"`
}

// PriceSide is one side of an onchain price.
type PriceSide struct {
	USD  string `json:"usd"`
	AtMs int64  `json:"atMs"`
}

// PairLiquidity is the exit-depth tier for a pair.
type PairLiquidity struct {
	Tier       string  `json:"tier"`
	Score      float64 `json:"score"`
	VenueCount int     `json:"venueCount"`
}

// MarketEvent is one frame from GET /tokens/market/socket.
type MarketEvent struct {
	Kind    string
	Time    int64
	ChainID int
	Address string

	HasBuy  bool
	Buy     string
	BuyAt   int64
	HasSell bool
	Sell    string
	SellAt  int64

	Settlement string
	Epoch      int64
	Block      int64

	StatusKind string
	Degraded   bool

	// Err is set when the connection dies. Kind is empty in that case.
	Err error
}
