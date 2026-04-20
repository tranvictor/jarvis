# jarvis

Ethereum automation made easy to human

## If you like Jarvis You can buy me a cup of coffee by sending any tokens to

0xe4d747cbdd6e8e5dd57db6735b6410a29f5027eb

Both Ethereum and BSC :)

## Installation

### MacOS via Homebrew

```bash
brew install tranvictor/jarvis/jarvis
```

or to upgrade jarvis to the latest version

```bash
brew upgrade jarvis
```

## Build from source

### Ubuntu Build

```bash
sudo add-apt-repository ppa:longsleep/golang-backports
sudo apt-get update
sudo apt-get install go-1.12
GO111MODULE=on /usr/lib/go-1.12/bin/go get github.com/tranvictor/jarvis@v0.0.1
GO111MODULE=on /usr/lib/go-1.12/bin/go install github.com/tranvictor/jarvis
```

`jarvis` command will be installed to `~/go/bin`

### MacOS Build

1. Download and install Go v1.12 [here](https://golang.org/dl/)

```
GO111MODULE=on go get github.com/tranvictor/jarvis
GO111MODULE=on go install github.com/tranvictor/jarvis
```

`jarvis` binary file will be placed at `$GOPATH/bin/`

If the installation process returned errors, try to clear Go module cache at `$GOPATH/pkg/mod`

2. Try `$GOPATH/bin/jarvis --help`
3. Add Jarvis to PATH, and relevant `addresses.json`

### Windows Build

Install mingw-w64 from [here](https://sourceforge.net/projects/mingw-w64/files/Toolchains%20targetting%20Win32/Personal%20Builds/mingw-builds/installer/mingw-w64-install.exe/download)
Add mingw-64 bin folder to PATH
Go to jarvis folder and using following command to build

```
go build -v
```

There should be jarvis.exe file. Add jarvis to PATH (optional)

Jarvis works on cmd, proshell but will not have color.
[Windows Terminal](https://www.microsoft.com/en-us/p/windows-terminal-preview/9n0dx20hk701?activetab=pivot:overviewtab) and [Gitbash](https://gitforwindows.org/) support color

## How to use it

See help with

```
~/go/bin/jarvis -h
```

## Multisig: Gnosis Classic and Gnosis Safe

Jarvis has first-class support for both Gnosis Classic (on-chain
confirmations, sequential `txid`s) and Gnosis Safe (off-chain / on-chain
confirmations, EIP-712 `safeTxHash`). **There is a single command for
both — `jarvis msig`.** Jarvis probes the address on-chain once, caches
the detected type per `(chain, address)` on disk, and dispatches to the
right backend automatically. You never have to tell jarvis which flavor
of multisig you're talking to.

> Older versions exposed a separate `jarvis safe` tree. It is gone. All
> Safe functionality lives under `jarvis msig` now.

### Subcommand matrix

| Subcommand | Classic | Safe | Notes |
|------------|:-------:|:----:|-------|
| `jarvis msig init`     | yes | yes | Propose a new multisig tx. |
| `jarvis msig approve`  | yes | yes | Add your approval. Auto-executes when threshold is met. |
| `jarvis msig execute`  | yes | yes | Broadcast the on-chain execution. |
| `jarvis msig info`     | yes | yes | Show a specific pending tx with decoded calldata. |
| `jarvis msig summary`  | yes | yes | List all pending txs for the multisig. |
| `jarvis msig gov`      | yes | yes | Show owners / threshold / version / nonce. |
| `jarvis msig bapprove` | yes | yes | Batch-approve many pending txs in one shot. Safe refs may be Safe-app URLs, `multisig_<safe>_<hash>` tokens, or `<chain>:<safe>:<hash>` triples. |
| `jarvis msig revoke`   | yes | **no** | Classic-only; errors with a clear message on Safe addresses. |
| `jarvis msig new`      | yes | **no** | Classic-only (deploys a new Classic wallet). |

`jarvis send --from <multisig>` also auto-detects and routes through
`msig init` automatically for both flavors, so you rarely need to reach
for `init` directly.

### Identifying a pending Safe transaction

Any of these work wherever a Safe pending-tx reference is expected:

```bash
# Safe-app URL (the easiest form to paste from the UI)
jarvis msig approve "https://app.safe.global/transactions/tx?id=multisig_0xSAFE_0xHASH&safe=eth:0xSAFE"

# Bare multisig token (as emitted by the Safe UI "Copy link")
jarvis msig approve multisig_0xSAFE_0xHASH

# Safe address + safeTxHash
jarvis msig approve 0xSAFE 0xHASH

# Safe address + SafeTx nonce
jarvis msig approve 0xSAFE 17
```

If you omit the identifier and there is exactly one pending Safe tx for
that wallet, jarvis auto-selects it.

### Safe approval modes

Two orthogonal approval paths are supported. You can mix them across
signers on the same pending tx — jarvis merges both sets at execute time.

1. **Off-chain (default).** Jarvis signs the EIP-712 `safeTxHash` with
   `--from` (or the only local owner wallet it finds) and POSTs the
   signature to the Safe Transaction Service.
2. **On-chain (`--approve-onchain`).** Jarvis broadcasts
   `Safe.approveHash(safeTxHash)` from `--from`. Useful on chains
   without a Transaction Service, for wallets that can't produce
   EIP-712 signatures, or when you want an on-chain audit trail.

```bash
# Off-chain (default): sign + post to Safe Transaction Service
jarvis msig approve 0xSAFE 0xHASH --from 0xMYOWNER

# On-chain: send approveHash(...) from --from
jarvis msig approve 0xSAFE 0xHASH --from 0xMYOWNER --approve-onchain
```

When your approval brings the signature count to the Safe's threshold,
jarvis **auto-chains `execTransaction` in the same invocation** so the
last signer doesn't need to run a second command. Pass `--no-execute` to
opt out.

### Chains without a Safe Transaction Service (`--safe-tx-file`)

Jarvis ships with Safe Transaction Service URLs for every chain where
Safe maintains one. On chains that don't have one (or when you prefer
not to use it), use a local JSON file as the source of truth:

```bash
# Proposer writes the SafeTx + their first signature to a file
jarvis msig init 0xSAFE --msig-to 0xTOKEN --msig-value 100 \
    --from 0xALICE --safe-tx-file ./proposal.json

# Other owners load the file, append their signature, write it back
jarvis msig approve 0xSAFE --from 0xBOB --safe-tx-file ./proposal.json

# Any owner executes from the file once threshold is met
jarvis msig execute 0xSAFE --from 0xCAROL --safe-tx-file ./proposal.json

# You can also just inspect a local proposal file
jarvis msig info 0xSAFE --safe-tx-file ./proposal.json
```

When `--safe-tx-file` is set, jarvis treats the file as the single
source of truth and does not consult the Safe Transaction Service even
if one is configured. You can still use `--approve-onchain` alongside a
file; the two approval paths merge at execute time via the Safe's
`approvedHashes` mapping.

If the service is up but you want to use a self-hosted deployment, set:

```
SAFE_TX_SERVICE_URL_<chainID>=https://my-safe-tx-service.example.com
# or a global fallback that applies to every chain:
SAFE_TX_SERVICE_URL=https://my-safe-tx-service.example.com
```

### Type-detection cache

The first time jarvis sees a `(chain, multisig-address)` pair it probes
the contract to decide Safe vs Classic and caches the answer on disk
(in `~/.jarvis/cache.json`) so subsequent commands don't pay the RPC
round-trip. Delete that file if you ever need to force re-detection
(e.g. after redeploying at the same address).

### Hardware wallet support

Ledger and Trezor are supported for both Classic confirmations and Safe
off-chain EIP-712 signing out of the box. Pick them through `--from`
exactly as you would for a normal `jarvis send`.

### A note on address inputs

Anywhere jarvis accepts an address (`--from`, `--msig-to`, the
multisig positional arg, etc.) it resolves the string in this order:

1. **ENS** — if the input looks like a `.eth` name (e.g. `alice.eth`,
   `foo.bar.eth`), jarvis resolves it against the canonical ENS
   registry on **Ethereum mainnet**, regardless of which chain you
   passed to `--network`. The same `0x…` is then used on the target
   chain. Results are cached in `~/.jarvis/cache.json` under the
   `ens:v1:<name>` key so subsequent runs don't re-query. If mainnet
   isn't configured or resolution fails, jarvis warns to stderr and
   falls through to step 2.
2. **Local address book** — built-in labels plus any entries you've
   added. Used both for forward lookup ("find an address by name") and
   for description tagging (printing `0xA0b8… (USDC - 6)`).
3. **Raw hex** — if the input contains a `0x…` hex address it's
   accepted as-is.

**Scope of ENS support:**

- `.eth` names only. Alt-TLDs that require CCIP-Read gateways
  (`alice.base.eth` on Basenames, `.linea.eth` on Linea Names,
  Unstoppable Domains, etc.) are **not** resolved. They'll fall
  through to the address book.
- Forward only. Jarvis does not reverse-resolve displayed addresses
  into `.eth` names; that would need a lookup on every rendered
  address and the cost isn't worth it for the CLI use case.
- EOA-safe, contract-caveat. EOAs have the same address on every EVM
  chain, so resolving `alice.eth → 0xABC` on mainnet and using `0xABC`
  as a signer on another chain is sound. Contract addresses are
  **not** guaranteed to host the same contract across chains — a
  multichain Safe with the same address on Ethereum and BSC is two
  independent Safes that happen to share an address. When an input
  like `--msig-to someproto.eth` resolves and you're operating on a
  non-mainnet chain, double-check the resolved `0x…` actually hosts
  the contract you expect.
- Results are shown with `ens:` as the provenance label (e.g.
  `To: 0xd8dA…6045 (ens:vitalik.eth)`) so you can always tell at a
  glance that an address came from ENS.

To disable the network hop entirely (airgap, offline signing), either
leave `~/.jarvis/nodes/mainnet.json` unconfigured (jarvis will warn
once and skip ENS for the rest of the run) or avoid typing `.eth`
names altogether.

## Ledger on Ubuntu

Add the rules and reload udev. More infomation see [here](https://support.ledger.com/hc/en-us/articles/115005165269-Fix-connection-issues)

```
wget -q -O - https://raw.githubusercontent.com/LedgerHQ/udev-rules/master/add_udev_rules.sh | sudo bash
```

## Configure custom nodes

Custom node is load from ~/nodes.json
This settings will override all default nodes
If any supported network is not define it will use default nodes

```
{
  "mainnet": {
    "infura": "infura_link",
    "alchemy": "alchemy_link"
  },
  "bsc": {
  }
}
```
