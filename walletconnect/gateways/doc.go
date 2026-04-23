// Package gateways provides concrete walletconnect.Gateway
// implementations for each wallet shape jarvis knows:
//
//   - EOA         : locally-held private key or hardware wallet
//   - Gnosis Safe : on-chain Safe contract with off-chain signatures
//   - Classic     : legacy Gnosis MultiSigWallet with submitTransaction
//
// Each gateway is a thin adapter: parsing/normalisation is already
// done by the session layer, terminal confirm prompts go through the
// injected UI, and device signing goes through jarvis's existing
// util/account infrastructure.
package gateways
