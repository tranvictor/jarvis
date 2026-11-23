package kyber

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const pageLimit = 1000
const maxPages = 30
const batchLimit = 100

// Chains returns the chains the data API serves.
func (c *Client) Chains(ctx context.Context) ([]Chain, error) {
	var data struct {
		Chains []Chain `json:"chains"`
	}
	if err := c.get(ctx, "/chains", nil, &data); err != nil {
		return nil, err
	}
	return data.Chains, nil
}

// Balances pages through every holding. chainID nil asks for every chain.
// Spam and honeypots are already removed by the API.
func (c *Client) Balances(ctx context.Context, wallet string, chainID *int) ([]Balance, error) {
	wallet = strings.TrimSpace(wallet)
	var all []Balance
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < maxPages; page++ {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(pageLimit))
		if chainID != nil {
			q.Set("chainId", strconv.Itoa(*chainID))
		}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var data struct {
			Balances   []Balance `json:"balances"`
			NextCursor string    `json:"nextCursor"`
		}
		path := "/wallets/" + url.PathEscape(wallet) + "/balances"
		if err := c.get(ctx, path, q, &data); err != nil {
			return nil, err
		}
		all = append(all, data.Balances...)
		next := strings.TrimSpace(data.NextCursor)
		if next == "" || next == cursor || len(data.Balances) == 0 || seen[next] {
			return all, nil
		}
		seen[next] = true
		cursor = next
	}
	return all, nil
}

// Tokens looks up catalog rows, at most 100 addresses per chain per call.
// Unknown tokens are omitted. A failed chunk is returned with whatever
// earlier chunks produced.
func (c *Client) Tokens(ctx context.Context, byChain map[int][]string) (map[int][]Token, error) {
	out := map[int][]Token{}
	var firstErr error
	for _, chunk := range chunkChains(byChain) {
		body := map[string][]string{}
		for id, addrs := range chunk {
			body[strconv.Itoa(id)] = addrs
		}
		var data map[string][]Token
		if err := c.post(ctx, "/tokens/batch", body, &data); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for id, tokens := range data {
			n, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			out[n] = append(out[n], tokens...)
		}
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, firstErr
}

// Market asks for onchain, settlement and window stats across chains.
func (c *Client) Market(ctx context.Context, byChain map[int][]string, windows []string) (map[int]ChainMarket, error) {
	out := map[int]ChainMarket{}
	var firstErr error
	for _, chunk := range chunkChains(byChain) {
		body := struct {
			Tokens  map[string][]string `json:"tokens"`
			Windows []string            `json:"windows"`
		}{Tokens: map[string][]string{}, Windows: windows}
		for id, addrs := range chunk {
			body.Tokens[strconv.Itoa(id)] = addrs
		}
		var data struct {
			Chains map[string]ChainMarket `json:"chains"`
		}
		if err := c.post(ctx, "/tokens/market", body, &data); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for id, m := range data.Chains {
			n, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			prev := out[n]
			if prev.Tokens == nil {
				prev.Tokens = map[string]TokenMarket{}
			}
			prev.Meta = m.Meta
			for addr, tm := range m.Tokens {
				prev.Tokens[strings.ToLower(addr)] = tm
			}
			out[n] = prev
		}
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, firstErr
}

// PairLiquidity returns the exit tier for tokenIn into tokenOut.
func (c *Client) PairLiquidity(ctx context.Context, chainID int, tokenIn, tokenOut string) (PairLiquidity, error) {
	q := url.Values{}
	q.Set("tokenIn", tokenIn)
	q.Set("tokenOut", tokenOut)
	var data PairLiquidity
	path := fmt.Sprintf("/liquidity/%d/pair", chainID)
	err := c.get(ctx, path, q, &data)
	return data, err
}

func chunkChains(byChain map[int][]string) []map[int][]string {
	var chunks []map[int][]string
	cur := map[int][]string{}
	for id, addrs := range byChain {
		for len(addrs) > 0 {
			n := batchLimit
			if n > len(addrs) {
				n = len(addrs)
			}
			cur[id] = append(cur[id], addrs[:n]...)
			addrs = addrs[n:]
			// One chain can fill a chunk on its own once it hits the cap.
			if len(cur[id]) >= batchLimit {
				chunks = append(chunks, cur)
				cur = map[int][]string{}
			}
		}
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}
