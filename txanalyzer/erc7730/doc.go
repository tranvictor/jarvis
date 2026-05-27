// Package erc7730 implements jarvis's Clear Signing layer based on
// ERC-7730 "Structured Data Clear Signing Format"
// (https://eips.ethereum.org/EIPS/eip-7730).
//
// # Overview
//
// ERC-7730 is a JSON descriptor format that sits beside the ABI. A
// descriptor binds itself to either:
//
//   - an EVM smart contract by (chainId, deployment-address) tuples, or
//   - an EIP-712 message by domain fields and/or domain separator
//
// and tells the wallet how to format each calldata parameter / message
// field into a human-readable line:
//
//   - "intent": a one-line summary of what the call does
//   - "interpolatedIntent": a templated string with {path} placeholders
//   - "fields": per-parameter formatters (tokenAmount, addressName,
//     date, enum, …) with a JSON-path style schema referring to the
//     decoded structured data (`#.`), the descriptor itself (`$.`),
//     or the transaction container (`@.`)
//
// # Architecture
//
// The package is intentionally side-effect free. It produces a
// ClearSignedView that the standard ui package renders inside a
// green-bordered box. The view sits above (not in place of) jarvis's
// existing ABI-decoded param table — descriptors enrich the display,
// they never hide raw data.
//
// Failure mode is fail-closed: any error (descriptor not found, path
// resolution failure, type mismatch, on-chain proxy lookup error, …)
// short-circuits to the existing display with no user-visible error.
// We never make signing review worse than it is today.
//
// # Trust model
//
// Treated like ENS resolution: a single trusted source (the
// ethereum/clear-signing-erc7730-registry GitHub repo) is fetched
// lazily on first miss, cached forever in
// ~/.jarvis/erc7730/registry/, and refreshed manually with
// `jarvis clearsign update`. User-added descriptors live alongside in
// ~/.jarvis/erc7730/local/ and override the registry on key conflict.
//
// The "visible: never" rule from the spec is honoured *visually*
// (those fields are not shown in the clear-signed block) but the raw
// ABI table below still shows everything — power-user CLI ethos says
// nothing gets hidden from the operator.
//
// The "mustMatch" rule is honoured as a hard fail: if a descriptor
// declares a field MUST be in an allowlist and the actual value
// isn't, the clear-signed render is aborted and a warning is shown,
// so signing falls back to the raw view.
package erc7730
