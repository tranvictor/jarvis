package erc7730

import (
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// FactoryDeployLookup is the caller-supplied hook for verifying a
// factory deployment: given the deploy-event signature topic and the
// factory's published deployments, return true if the contract at
// `addr` was emitted by one of the factory's deploy events on
// `chainID`. The implementation typically scans the transaction
// receipt log history via a block-explorer query.
//
// Provided as a hook so the erc7730 package stays free of explorer
// dependencies; clearsign.go wires it to util/jarvisutil at runtime.
type FactoryDeployLookup func(chainID uint64, addr string, deployEventSig string, factories []Deployment) bool

// MatchesFactory checks whether contract addr is a known
// factory-deployed instance per descriptor's context.contract.factory
// binding. Returns true on first matching factory; false when no
// factory is set or lookup says no.
//
// The deploy-event signature is the first argument to keccak256 —
// e.g. "PoolCreated(address indexed pool,address tokenA,address tokenB)"
// hashes to the canonical event topic. The spec defines the binding
// as "the first indexed address argument of the event is the
// deployed contract"; verifying that requires reading event topics
// from the explorer, which is delegated to FactoryDeployLookup.
func MatchesFactory(d *Descriptor, chainID uint64, addr string, lookup FactoryDeployLookup) bool {
	if d == nil || d.Context.Contract == nil || d.Context.Contract.Factory == nil {
		return false
	}
	if lookup == nil {
		return false
	}
	f := d.Context.Contract.Factory
	return lookup(chainID, strings.ToLower(addr), f.DeployEvent, f.Deployments)
}

// EventTopic returns the keccak256 topic for a Solidity-format event
// signature (e.g. "PoolCreated(address,address,address,uint24,address)").
// The spec uses the human-readable form (with parameter names) for the
// `deployEvent` key — strip the names first; reuse the existing
// parseFormatKey helper because the syntax is identical.
func EventTopic(eventSig string) common.Hash {
	parsed, err := parseFormatKey(eventSig)
	if err != nil {
		return common.Hash{}
	}
	canonical := parsed.Name + "(" + strings.Join(parsed.ParamTypes, ",") + ")"
	return common.BytesToHash(crypto.Keccak256([]byte(canonical)))
}
