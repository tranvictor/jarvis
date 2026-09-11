# WalletConnect: drive dApps from jarvis (`jarvis wc`)

Browser-hosted dApps such as AAVE, KyberSwap and Uniswap all speak
[WalletConnect v2](https://docs.walletconnect.com/). `jarvis wc`
turns jarvis into a WalletConnect **wallet** for the duration of one
command, so you can sit at a terminal and have the dApp propose
transactions for any jarvis-managed account — EOAs, Gnosis Safes, or
legacy Gnosis classic multisigs alike.

## Quick start

```
# 1. On the dApp: pick "WalletConnect" in the connect dialog,
#    then "Copy URI" (looks like `wc:7a9c...@2?relay-protocol=...`).

# 2. Paste it into jarvis and pick the account you want to act as:
jarvis wc "wc:7a9c...@2?relay-protocol=irn&symKey=..." \
    --from alice.eth
```

Jarvis ships with a bundled Reown projectId so the above works with
no extra setup. See [projectId](#projectid) below if you want to
register your own.

jarvis then:

1. Pairs with the dApp using the URI's symmetric key.
2. Shows you the incoming session proposal (dApp name, URL, requested
   chains and methods) and asks you to confirm.
3. Subscribes to the session topic and **blocks**. Every
   `eth_sendTransaction`, `personal_sign`, `eth_signTypedData_v4`, or
   `wallet_switchEthereumChain` the dApp emits is printed with
   jarvis's usual decoded-calldata view before you get a
   sign/broadcast prompt. Rejecting a request sends the dApp a
   JSON-RPC error; the session stays open for the next one.
4. Exits cleanly on `Ctrl-C` (jarvis tells the dApp
   `wc_sessionDelete` before disconnecting, so the dApp UI updates
   immediately).

## How `--from` picks the signing strategy

`jarvis wc` reuses the same type-detection pipeline as `jarvis msig`
and `jarvis send`, so you can point it at any of:

- **EOA** (`--from alice.eth`, `--from 0xabc…`, or a wallet nickname)
  — jarvis signs and broadcasts each `eth_sendTransaction` directly.
  `personal_sign` / `eth_signTypedData_v4` / `wallet_switchEthereumChain`
  all work.
- **Gnosis Safe** (`--from <safe-addr>`) — each `eth_sendTransaction`
  is wrapped into a SafeTx, signed with your configured owner, and
  posted to the Safe Transaction Service. You then collect the other
  owners' signatures with `jarvis msig approve`/`execute` as usual.
  Raw signing methods are rejected, because there is no safe,
  generic way for a Safe to counter-sign an arbitrary off-chain
  message.
- **Gnosis Classic multisig** (`--from <classic-msig-addr>`) — each
  `eth_sendTransaction` is wrapped into `submitTransaction(...)` and
  broadcast by your owner EOA. Other owners confirm the resulting
  on-chain transaction ID with `jarvis msig approve`. Raw signing
  methods are rejected.

For multisig `--from`, jarvis auto-picks a local wallet that is also
an on-chain owner. If several of your local wallets qualify, pass
`--owner <addr>` to disambiguate.

## `--network` and `wallet_switchEthereumChain`

`--network` sets the **default** chain jarvis will honour; any dApp
chain outside of that is rejected unless you pre-authorise it. If the
dApp issues `wallet_switchEthereumChain`:

- **EOA**: jarvis swaps its node reader/broadcaster to the requested
  chain. The same EOA address is used (EOAs are address-stable
  across EVM chains).
- **Gnosis Safe**: jarvis checks the Safe Transaction Service
  registry (same data powering the chain autodetect for `jarvis
  msig`) to confirm the target chain actually has that Safe
  deployed; refuses otherwise.
- **Gnosis Classic**: jarvis probes the target chain for the
  `submitTransaction` / `confirmTransaction` / `getTransactionCount`
  selector surface; refuses if the target chain has no classic
  multisig code at that address.

## projectId

The WalletConnect relay requires every client to identify itself
with a registered Reown projectId (used for quota accounting, not
authentication of secrets). Jarvis ships with a bundled default
projectId so the out-of-the-box experience works without any signup.
If you share that default with the rest of the jarvis userbase you
may occasionally hit relay rate limits — in that case register your
own at [cloud.reown.com](https://cloud.reown.com) and either export
`JARVIS_WC_PROJECT_ID=<your-id>` or pass `--project-id <your-id>`.
No other WalletConnect credentials are stored; the session ephemeral
keys are generated in-memory for every run.
