package util

import (
	"context"
	"math/big"
	"time"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/db"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	jarvisutil "github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/vet"
	"github.com/tranvictor/jarvis/vet/ai"
)

func attachVet(card *SigningCard, warn WarningInput, network jarvisnetworks.Network) {
	req := vetRequest(warn, network, false, nil)
	runVet(card, req)
}

func attachVetCreate(card *SigningCard, warn WarningInput, network jarvisnetworks.Network, data []byte) {
	req := vetRequest(warn, network, true, data)
	runVet(card, req)
}

func runVet(card *SigningCard, req vet.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	report := vet.Analyze(ctx, req)
	card.Vet = report.Findings
}

func vetRequest(warn WarningInput, network jarvisnetworks.Network, create bool, data []byte) vet.Request {
	req := vet.Request{
		Mode:         vet.ModeAlways,
		ChainID:      network.GetChainID(),
		NetworkName:  network.GetName(),
		To:           warn.To,
		Value:        warn.Value,
		Data:         data,
		Call:         warn.Call,
		DelegateCall: warn.DelegateCall,
		MultiSend:    warn.MultiSend,
		Create:       create,
	}
	if warn.HasData && warn.Call != nil {
		req.Data = warn.Call.Data
	}
	if !config.Careful {
		return req
	}
	req.Mode = vet.ModeFull
	req.Book = loadBook()
	req.Chain = explorerLookup(network)
	req.AI = grokCompleter()
	return req
}

// AddressBook is the local hex→label map used by poisoning and typed-data
// spender checks. Labels are never sent to Grok.
func AddressBook() []vet.BookAddr {
	return loadBook()
}

func loadBook() []vet.BookAddr {
	var out []vet.BookAddr
	for hex, label := range db.AllAddresses() {
		out = append(out, vet.BookAddr{Hex: hex, Label: label})
	}
	return out
}

// ExplorerLookupFor is the production Lookup used by `jarvis vet`.
func ExplorerLookupFor(network jarvisnetworks.Network) vet.Lookup {
	return explorerLookup(network)
}

// GrokCompleter is the production Completer (nil when XAI_API_KEY is unset).
func GrokCompleter() vet.Completer {
	return grokCompleter()
}

func explorerLookup(network jarvisnetworks.Network) vet.Lookup {
	r, err := jarvisutil.EthReader(network)
	if err != nil {
		return nil
	}
	return vet.ExplorerLookup{Explorer: network, Reader: r}
}

func grokCompleter() vet.Completer {
	c := ai.NewFromEnv()
	if c.Key == "" {
		return nil
	}
	return vet.EnvCompleter{Client: c}
}

// PrintVetReport writes findings the same way the signing card does.
func PrintVetReport(u ui.UI, report vet.Report) {
	if len(report.Findings) == 0 {
		u.Success("vet: no findings")
		return
	}
	for _, f := range report.Findings {
		text := f.Text
		if f.GrokReconfirm {
			text += " — Grok reconfirms"
		}
		if f.Risk == vet.RiskDanger {
			u.Error("! %s", text)
		} else {
			u.Warn("! %s", text)
		}
	}
}

// FullVetRequest is used by `jarvis vet` (always ModeFull).
func FullVetRequest(
	network jarvisnetworks.Network,
	to jarviscommon.Address,
	value *big.Int,
	data []byte,
	fc *jarviscommon.FunctionCall,
) vet.Request {
	req := vet.Request{
		Mode:        vet.ModeFull,
		ChainID:     network.GetChainID(),
		NetworkName: network.GetName(),
		To:          to,
		Value:       value,
		Data:        data,
		Call:        fc,
		Book:        loadBook(),
		Chain:       explorerLookup(network),
		AI:          grokCompleter(),
	}
	if to.Address == "" {
		req.Create = true
	}
	return req
}
