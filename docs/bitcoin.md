# Bitcoin (`jarvis btc`)

Native Bitcoin support, separate from the EVM commands. Wallets are native
segwit (P2WPKH, BIP-84, `bc1q…`); the destination can be any standard address
type (P2PKH, P2SH, P2WPKH, P2WSH, P2TR).

```
jarvis btc wallet add --kind keystore --keystore ~/.jarvis/foo.json   # same file as an EVM wallet
jarvis btc wallet add --kind privatekey                               # hex or WIF, asked on the terminal
jarvis btc wallet add --kind trezor [--account N | --path "m/84'/0'/N'"]
jarvis btc wallet add --kind ledger [--account N]
jarvis btc wallet list
jarvis btc balance --from <wallet|address>
jarvis btc fees
jarvis btc send --from <wallet> --to <address> --amount 0.001 [--fee-rate 12 | --target 2] [--no-broadcast]
```

`--chain mainnet|testnet|signet|regtest` picks the chain when a wallet is
added; wallets remember it. `--amount` takes BTC, `150000 sat` or `ALL`
(sweep; the fee comes out of the amount).

Wallet records live in `~/.jarvis/btc/` (no secrets: keystore path, or the
account xpub and master fingerprint for hardware wallets). The EVM wallet
store (`~/.jarvis/*.json`) is not read or written.

## Behaviour worth knowing

- Only confirmed coins are spent unless `--spend-unconfirmed` is given.
- Inputs signal RBF (sequence `0xfffffffd`). Change goes to the next unused
  internal address (hardware wallets) or back to the wallet address.
- Fee = ceil(rate x estimated vsize), never below 1 sat/vB. Rates above
  `--max-fee-rate` (default 500) are refused; a fee above 10% of the
  payment is flagged with a warning on the confirmation card.
- Change below the dust limit is added to the fee instead of creating an
  output. Dust limits are per output type (P2WPKH 294 sat, P2PKH 546 sat, ...).
- Hardware wallets: the host checks the device's master fingerprint and
  account key against the wallet record before signing, rebuilds every
  witness from the returned signatures and verifies pubkey, SIGHASH_ALL and
  one signature per input. The device screen is the place to check the
  destination, amount and fee.
- Trezor: prior transactions are fetched from Esplora and checked against
  their txid before being handed to the device. Ledger: needs the Bitcoin app
  (new-protocol app; tested with 2.4.2; "Bitcoin Test" for non-mainnet chains), signing goes through
  `sign_psbt` with a registered-by-default `wpkh(@0/**)` policy.
- Rejecting on a device exits non-zero and says nothing was broadcast.
  Answering "no" at jarvis's own confirmation prompt also sends nothing, but
  exits 0.

## Failure modes and the scenario that exercises each

| Failure | Scenario |
|---|---|
| Bitcoin records leak into / break the EVM wallet store | S01 |
| Wrong keystore passphrase creates a record; wrong password lets a send proceed | S02, S12 |
| Keystore replaced by another key after the wallet was added | S13 |
| Wrong-chain WIF / garbage key accepted | S03 |
| Fee estimate missing and a made-up fee is used | S04 |
| Pending coins counted as spendable; watch-only / wrong-chain address | S05 |
| Invalid signature, wrong BIP-143 sighash, wrong change, wrong fee | S06 (bitcoind validates) |
| Unit parsing (`sat`, 9 decimals, negatives, exponents) | S07, S08 |
| Absurd fee rates; fee larger than the payment | S09 |
| `--no-broadcast` leaks a transaction / produces an invalid one | S10 |
| User declines and something is still sent | S11 |
| Unconfirmed change spent silently; invalid chained spend | S14 |
| Dust change created; sweep leaves dust | S15 |
| Indexer outage or garbage reported as success or guessed around | S16 |
| Stale UTXO list (coin already spent) reported as sent | S17 |
| `--target` ignored | S18 |
| Destination type unsupported; wrong dust limit per type | S19 |
| Ledger: addresses differ from BIP-84; wrong app/network prefix | L01 |
| Ledger: multi-input sign, change not recognised by device | L02, L03 |
| Ledger: reject on device still broadcasts | L04 |
| Ledger: different device / seed than the wallet | L05 |
| Ledger: sweep with many inputs; unplugged device | L06, L07 |
| Trezor: same list as the Ledger one, plus offline balance | T01-T07 |

Not covered by any automated scenario: physical USB transports (Ledger HID
framing, Trezor USB), device PIN and passphrase prompts for Bitcoin messages,
mainnet, Esplora instances other than the regtest shim.

## E2E test

```
make e2e-btc            # or scripts/e2e-btc.sh
```

Starts a real `bitcoind -regtest` (Bitcoin Core 28.0, sha256 pinned in the
script) behind a small Esplora-compatible shim (`btc/e2e/shim`), builds
`jarvis`, and drives its CLI. Every broadcast is validated by bitcoind. The
report (`btc-e2e-report.md` and `.json`, every command and output) goes to
`btc/e2e/artifacts/` or `$ARTIFACTS`. It fails if `bitcoind` cannot be
obtained.

Hardware scenarios use the real firmware / app in emulators and report
`NOT RUN` unless configured (`E2E_REQUIRE_TREZOR=1` / `E2E_REQUIRE_LEDGER=1`
turn that into a failure):

| Variable | Meaning |
|---|---|
| `TREZOR_FIRMWARE_DIR`, `TREZOR_PYTHON` | trezor-firmware checkout with the emulator built, and its venv python |
| `SPECULOS_BIN`, `LEDGER_APP_ELF`, `SPECULOS_API_LEVEL` | Speculos, the Bitcoin Test app ELF, API level (default 27) |

### Building the emulators (what worked on Ubuntu 24.04, x86_64)

Trezor: build SDL3 and SDL3_image from source into a prefix (the distro has
none) and export `LD_LIBRARY_PATH=<prefix>/lib` when running; install
`libstdc++-14-dev`; use Rust `nightly-2026-03-16` (newer nightlies break the
firmware's `impl const` syntax); build with `CC=gcc CXX=g++` (clang rejects
`_Float16` under `-Werror`); build the emulator with the **test** preset
(`-p test`), otherwise the debug link used to press buttons is missing.
`btc/e2e/trezor/autoconfirm.py` is the debug-link helper.

Ledger: clone `LedgerHQ/app-bitcoin-new` (tag 2.4.2) and the Ledger secure SDK,
install `lld`, `qemu-user-static` and `pip install speculos pillow`, then
`make BOLOS_SDK=<sdk> TARGET=nanos2 API_LEVEL=27 COIN=bitcoin_testnet` with
`C_INCLUDE_PATH=<app>/src` and the pip venv on `PATH`. Run Speculos with
`-a 27 --model nanosp`. `rm -rf build` between coin variants; `make clean` does
nothing without `BOLOS_SDK`.

Environment variables for development against an emulator or a node:
`JARVIS_BTC_ESPLORA` (indexer URL), `JARVIS_TREZOR_UDP=host:port` (Trezor
emulator), `JARVIS_LEDGER_SPECULOS=host:port` (Speculos APDU port),
`JARVIS_BTC_DEVICE_WAIT` (how long to wait for a device, e.g. `5s`).
