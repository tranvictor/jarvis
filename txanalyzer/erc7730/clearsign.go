package erc7730

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

// ClearSignedView is the renderable summary of a successful clear-signing
// match. It is produced by ContractView / EIP712View and consumed by
// Render in render.go. Everything is plain text — the ui layer paints
// the colours.
type ClearSignedView struct {
	// Intent is the descriptor's `intent` field, possibly multi-line
	// when intent is a key/value object.
	Intent []string
	// InterpolatedIntent is the rendered "Send 100 USDC to alice.eth"
	// line — already filled in from field values.
	InterpolatedIntent string
	// Fields is the field-by-field rendered table content.
	Fields []FormattedField
	// Source describes where the descriptor came from (e.g.
	// "registry" or "local").
	Source string
	// CachedAt is the descriptor file's mtime as a unix timestamp,
	// used to print "cached YYYY-MM-DD" alongside Source.
	CachedAt time.Time
	// Owner / ContractName are surfaced verbatim from metadata so the
	// box header can carry "Approve · Uniswap V3 Router" rather than
	// the raw destination address.
	Owner        string
	ContractName string
	// Warning is set when something looked off — descriptor failed
	// mustMatch, a path didn't resolve, etc. The render layer shows
	// the warning in yellow above the field table.
	Warning string
}

// Engine ties together a descriptor source, the matchers, the
// formatters, and the optional Helpers. One Engine per process is
// fine; everything inside is stateless or guarded by the registry's
// own locks.
type Engine struct {
	Source DescriptorSource
	// Helpers wires the formatters to jarvis's address-book / ERC-20
	// /  network lookups. May be nil — formatters degrade to raw
	// representations in that case.
	Helpers Helpers
	// ImplFor optionally resolves a proxy address to its
	// implementation. May be nil (we then just fail the lookup for
	// proxy contracts not in the registry).
	ImplFor func(addr string) (string, bool)
	// AutoSyncEvery controls "lazy fetch on miss" — when a contract
	// lookup fails and the LocalRegistry hasn't been synced in at
	// least this duration, the engine triggers a background sync
	// and retries the lookup. Zero disables auto-sync.
	AutoSyncEvery time.Duration
}

// ContractView produces a ClearSignedView for an EVM transaction.
// Returns (nil, nil) when no descriptor applies — callers MUST treat
// that as "show the existing raw view, no clear-sign block".
func (e *Engine) ContractView(
	ctx context.Context,
	chainID uint64,
	to string,
	value *big.Int,
	calldata []byte,
	params []jarviscommon.ParamResult,
	contractABI *abi.ABI,
) (*ClearSignedView, error) {
	if len(calldata) < 4 {
		return nil, nil
	}

	in := ContractMatchInput{
		ChainID:  chainID,
		To:       strings.ToLower(to),
		Calldata: calldata,
		ImplFor:  e.implForChain(chainID),
	}
	matched := FindContractMatch(e.Source, in)
	if matched == nil {
		matched = e.lazySyncAndRetryContract(ctx, in)
	}
	if matched == nil {
		return nil, nil
	}

	data := BuildDataFromParams(params)
	// Prefer a direct ABI unpack when the caller supplies the
	// contract ABI — this mirrors go-ethereum's struct binding and
	// avoids subtle name/shape drift in the ParamResult conversion.
	if contractABI != nil {
		if method, err := contractABI.MethodById(calldata[:4]); err == nil {
			if decoded, err := buildDataFromCalldata(matched, calldata, method); err == nil {
				data = decoded
			}
		}
	}

	// Align top-level param names with the descriptor format key.
	// Verified ABIs often use compiler-internal names (_execution)
	// that differ from the descriptor's declared names (execution).
	data = applyParamNames(data, matched.ParamNames)

	resolver := &Resolver{
		Data:       data,
		Descriptor: matched.Descriptor,
		Container: Container{
			From:    "", // populated upstream from the tx envelope; not always known here
			To:      strings.ToLower(to),
			Value:   safeBig(value),
			ChainID: chainID,
		},
	}
	return e.buildView(resolver, matched), nil
}

// EIP712View produces a ClearSignedView for an eth_signTypedData_v4
// request. Returns (nil, nil) when no descriptor applies.
func (e *Engine) EIP712View(
	ctx context.Context,
	td *apitypes.TypedData,
	signer string,
) (*ClearSignedView, error) {
	matched := FindEIP712Match(e.Source, EIP712MatchInput{TypedData: td})
	if matched == nil {
		return nil, nil
	}

	data := buildDataFromTypedData(td, matched.ParamNames)
	resolver := &Resolver{
		Data:       data,
		Descriptor: matched.Descriptor,
		Container: Container{
			From:    strings.ToLower(signer),
			To:      strings.ToLower(td.Domain.VerifyingContract),
			Value:   new(big.Int),
			ChainID: domainChainID(td),
		},
	}
	return e.buildView(resolver, matched), nil
}

// buildView is the shared formatter loop used by ContractView and
// EIP712View. The resolver is fully populated; we just walk fields
// and assemble the rendered rows.
func (e *Engine) buildView(resolver *Resolver, matched *MatchedFormat) *ClearSignedView {
	view := &ClearSignedView{
		Source:       matched.Descriptor.Source,
		CachedAt:     time.Unix(matched.Descriptor.CachedAtUnix, 0),
		Owner:        matched.Descriptor.Metadata.Owner,
		ContractName: matched.Descriptor.Metadata.ContractName,
	}

	// Intent (string or map).
	if intent := matched.Format.Intent; !intent.IsEmpty() {
		switch {
		case intent.Map != nil:
			for k, v := range intent.Map {
				view.Intent = append(view.Intent, k+": "+v)
			}
		case intent.Text != "":
			view.Intent = []string{intent.Text}
		}
	}

	fmtr := &Formatter{Resolver: resolver, Helpers: e.Helpers}

	// Enforce mustMatch first — those failures abort the whole view.
	if violation := enforceMustMatch(matched.Format, resolver); violation != "" {
		view.Warning = violation
		return view
	}

	// Render fields.
	for _, field := range matched.Format.Fields {
		rows, err := fmtr.FormatField(field)
		if err != nil {
			// Non-fatal: skip the row, leave a debug breadcrumb in
			// the warning string so the user can see something
			// went wrong.
			if view.Warning == "" {
				view.Warning = fmt.Sprintf("formatter error on %s: %s", field.Path, err.Error())
			}
			continue
		}
		view.Fields = append(view.Fields, rows...)
	}

	// Interpolated intent: render *after* fields so we can re-use
	// the field formatter for {path} placeholders.
	if matched.Format.InterpolatedIntent != "" {
		view.InterpolatedIntent = interpolate(
			matched.Format.InterpolatedIntent, matched.Format.Fields, fmtr,
		)
	}

	return view
}

// applyParamNames aligns the resolver's top-level tuple field names
// with those declared in the descriptor's format key. The spec
// mandates that paths like `#.execution.desc.amount` use the format
// key's parameter names, not necessarily the compiler's internal ABI
// names (_execution, etc.). When the ABI omitted names entirely we
// still fill them in; when the ABI named them differently the
// descriptor wins as long as the arity matches.
func applyParamNames(data ResolvedValue, names []string) ResolvedValue {
	if data.Kind != ResolvedTuple {
		return data
	}
	if len(data.Tuple) != len(names) {
		return data
	}
	out := ResolvedValue{Kind: ResolvedTuple}
	for i, f := range data.Tuple {
		nm := f.Name
		if i < len(names) && names[i] != "" {
			nm = names[i]
		}
		out.Tuple = append(out.Tuple, ResolvedField{Name: nm, Value: f.Value})
	}
	return out
}

// enforceMustMatch returns a non-empty warning string when any field
// declared visible: { mustMatch: [...] } resolves to a value outside
// the allow-list. mustMatch is a security gate: the descriptor is
// telling the wallet "ABORT if this value isn't one of these", so we
// surface the failure to the operator.
func enforceMustMatch(f *Format, r *Resolver) string {
	for _, field := range f.Fields {
		if field.Visible.Kind != VisibilityMustMatch || field.Path == "" {
			continue
		}
		p, err := ParsePath(field.Path)
		if err != nil {
			continue
		}
		val, err := r.Resolve(p)
		if err != nil {
			continue
		}
		got := rawStringForVisibility(val)
		ok := false
		for _, expected := range field.Visible.Values {
			if strings.EqualFold(expected, got) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Sprintf("descriptor mustMatch failed: %s = %q not in %v",
				field.Path, got, field.Visible.Values)
		}
	}
	return ""
}

// implForChain returns proxy→implementation resolution scoped to a
// single chain. The legacy Engine.ImplFor hook scanned every jarvis
// network and could stall tx confirmation on slow RPC nodes; chain
// scoping keeps the best-effort proxy fallback to one round trip.
func (e *Engine) implForChain(chainID uint64) func(string) (string, bool) {
	if h, ok := e.Helpers.(interface {
		ImplementationOf(addr string, chainID uint64) (string, bool)
	}); ok {
		return func(addr string) (string, bool) {
			return h.ImplementationOf(addr, chainID)
		}
	}
	return e.ImplFor
}

// lazySyncAndRetryContract refreshes the upstream registry when we
// have no descriptor bound to (chainID, to) and the local mirror is
// older than AutoSyncEvery. A selector mismatch against a known
// binding is not a sync candidate — refreshing won't add formats.
func (e *Engine) lazySyncAndRetryContract(ctx context.Context, in ContractMatchInput) *MatchedFormat {
	lr, ok := e.Source.(*LocalRegistry)
	if !ok || e.AutoSyncEvery <= 0 {
		return nil
	}
	if len(lr.FindByContract(in.ChainID, in.To)) > 0 {
		return nil
	}
	if lr.LastSyncAge() < e.AutoSyncEvery {
		return nil
	}
	if _, err := lr.SyncRegistry(ctx, SyncOptions{}); err != nil {
		return nil
	}
	lr.TouchLastSync()
	return FindContractMatch(e.Source, in)
}

func safeBig(b *big.Int) *big.Int {
	if b == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(b)
}
