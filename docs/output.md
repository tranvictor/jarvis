# Reading the output

Jarvis prints the things you have to decide on first and the things you
might want to look up later further down. Colour is used sparingly: green
for a confirmed outcome, red for a failure, yellow only for something you
should read before signing. Addresses show their address-book name first
and a shortened hex (`0x9642…5D4E`); the full hex is always shown on
signing screens and with `--degen`. `--json-output` is unaffected by any of
this and always carries full, untruncated values.

## `jarvis info <hash>`

```
✓ done   swapExactTokensForTokens  →  Uniswap V2 Router (0x7a25…488D)
         mainnet   from me (0x9642…5D4E)   value 0 ETH   gas 0.00213000 ETH   nonce 412   block 19234567

Call  swapExactTokensForTokens  →  Uniswap V2 Router (0x7a25…488D)
  amountIn      1,000,000,000  uint256
  amountOutMin  311,200,000,000,000,000  uint256
  path          [2 items]  address[]
  ├─ USDC (0xA0b8…eB48)
  └─ WETH (0xC02a…6Cc2)
  to            me (0x9642…5D4E)  address
  deadline      2024-09-06 05:20:00 UTC, 2 years ago  uint256

Events (3)
  1. Transfer  USDC token (0xA0b8…eB48)   from me (0x9642…5D4E)  to USDC/WETH pair (0x0d4a…1852)  value 1,000 USDC
  2. Sync      USDC/WETH pair (0x0d4a…1852)   reserve0 5,000,000,000,000  reserve1 1,500,000,000,000,000,000,000
  3. Transfer  WETH token (0xC02a…6Cc2)   from USDC/WETH pair (0x0d4a…1852)  to me (0x9642…5D4E)  value 0.3121 WETH

✓ done   swapExactTokensForTokens  →  Uniswap V2 Router (0x7a25…488D)   0x3f9a…e1c2
```

- The headline is status + what was called + where. The muted second line
  holds the numbers you rarely need (network, gas, nonce, block).
- Time-named parameters (`deadline`, `expiry`, `validUntil`, …) show the
  date and distance instead of raw seconds; `-x` keeps both.
- With four or more token movements a **Net effect** block comes first: one
  line per address with its net change per token, sender first, so a swap
  that hops through three pools still reads as "−1,000 USDC, +0.3121 WETH".
  The per-hop list is not printed; **Events** at the bottom has every
  `Transfer` / `Approval` / `Deposit` / `Withdrawal`, including standard
  token events from unverified contracts. `--json-output` still includes
  a `transfers` array.
- Unknown addresses are shown as bare short hex; `(zero address)` marks
  mints, burns and empty approve-targets, and is never replaced by an
  address-book name. Integers get thousands separators and token amounts are
  rounded to four decimals (four significant digits below 1). `--degen` and
  `--json-output` keep every digit.
- Arrays longer than a few items and long `bytes` blobs are collapsed;
  `--degen` expands everything, shows full addresses and switches the
  parameter list to a table.
- A reverted tx opens with `✗ reverted`; the revert reason, when it can be
  recovered by replaying the call, is the red `reason` line under the
  headline. Events that no ABI describes are listed as `<undecoded>` with
  their raw topics so the event count is always complete.
- When an ERC-7730 descriptor matches the call (or an inner MultiSend / Safe
  call), a green **Clear Signed** panel is printed above the ABI **Call**
  tree — the same fields shown at sign time, without the hardware-wallet
  comparison hint. No match leaves the output unchanged.

## Signing screen

Every `send`, `tx`, `msig init/approve/execute`, Classic and Safe
`msig info`, and WalletConnect `eth_sendTransaction` ends in the same
card. WalletConnect `personal_sign` / typed-data still use a compact
confirm. The decoded call comes first (with a green Clear Signed panel
above it when an ERC-7730 descriptor matches), the who/where/cost block sits
directly above the prompt, and anything jarvis thinks you should
double-check is listed as a `!` line right before you answer:

```
================ EOA transaction =================

Call  approve  →  0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48 (USDC)
  spender  0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D  address
  amount   uint256.max (∞)  uint256

Sign with  0x9642b23Ed1E01Df1092B92641051881a322F5D4E (me)   ledger   mainnet
Send to    0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48 (USDC)
Gas        ≈ 0.00170246 ETH   (85,123 gas × max 20 gwei, tip 1.5 gwei)   nonce 42

! approves UNLIMITED USDC to 0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D
! spender 0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D is not in your address book

Sign and broadcast (≈ 0.00170246 ETH)? [y/N]
>
```

```
================ Safe approval =================

Send  1.5 ETH  →  0x9642b23Ed1E01Df1092B92641051881a322F5D4E (Alice)

Sign with    0x9642b23Ed1E01Df1092B92641051881a322F5D4E (me)   ledger   mainnet
Recipient    0x9642b23Ed1E01Df1092B92641051881a322F5D4E (Alice)
Value        1.5 ETH
Operation    CALL (0)
Safe nonce   17
safeTxHash   0xabc…

Sign this Safe approval (off-chain, no gas)? [y/N]
>
```

An ERC-20 transfer uses the same `Send  amount  →  recipient` line (the
token contract stays on `Safe calls` / `Send to`). Attaching native value
to an ERC-20 `transfer` / `approve` / similar call adds:

```
! attaches 1.5 ETH to an ERC-20 transfer on USDC; the native value goes to the token contract, not the recipient
```

WETH `deposit` is payable on purpose, so it keeps the ordinary
`sends 1.5 ETH into a contract` warning instead.

Warnings are emitted for: a destination that is not in your address book,
native value sent into a contract, native value attached to an ERC-20 call,
calldata jarvis could not decode,
unlimited ERC-20 approvals, `setApprovalForAll`, approvals to unknown
spenders, and Safe `DELEGATECALL` operations. Safe cards add the Safe
address, operation, nonce, `safeTxHash` and the list of collected
signatures. Classic cards add the multisig, on-chain tx id, confirmation
progress and the list of confirmers; approving then collapses the inner
`confirmTransaction` call the same way a Safe execution collapses
`execTransaction`. `-Y` / `--yes` skips the prompt but still prints the card.

After signing, a live status line replaces the silent wait
(`⠋ in mempool, waiting to be mined…  0:12`), and once mined jarvis prints
the same headline + Net effect + Events view as `info` so you can see what
actually happened. The same status line is used while jarvis waits for a
Ledger to be plugged in and unlocked.

## Batch runs

`jarvis msig bapprove` shows the plan before asking anything, then works
through the list with one indented block per item. The inner Classic/Safe
operation is a rounded box so it stands apart from the EOA confirm that
follows. Every banner, box title, Y/n prompt and result line repeats
`[i/n]`, so you can search or scroll a 50-item transcript and still know
which transaction you are looking at:

```
=============== Batch approve: 3 transaction(s) ===============
1. Safe     eth:0xSafe…:0xhash…
2. Safe     bsc:0xSafe…:0xhash…
3. Classic  mainnet:0xinit…

=============== [1/3] Safe  eth:0xSafe…:0xhash… ===============
  ╭─ [1/3] Safe approval ──────────────────────────────╮
  │ ... decoded call, warnings ...                     │
  ╰────────────────────────────────────────────────────╯
  [1/3] Sign approval (off-chain, no gas)? [Y/n]
✓ [1/3] approved  safeTxHash 0x…   (1 ok · 2 left)

=============== [2/3] Safe  bsc:0xSafe…:0xhash… ===============
  ...
✗ [2/3] failed  unlock wallet: ledger not connected   (1 ok · 1 failed · 1 left)
Continue with the remaining 1 transaction(s)? [Y/n]

=============== [3/3] Classic  mainnet:0xinit… ===============
  ╭─ [3/3] Classic multisig transaction ───────────────╮
  │ Send  1.5 ETH  →  Alice                            │
  │ ...                                                │
  ╰────────────────────────────────────────────────────╯
  [3/3] EOA transaction
  Call  confirmTransaction  →  Treasury   (Classic transaction shown above)
  [3/3] Sign and broadcast (≈ 0.0017 ETH)? [Y/n]
✓ [3/3] approved  confirm tx 0x…   (2 ok · 1 failed)

===================== Batch summary =====================
┌───┬─────────┬─────────┬──────────┬──────────┬───────────────────────┐
│ # │ Kind    │ Network │ Target   │ Result   │ Detail                │
├───┼─────────┼─────────┼──────────┼──────────┼───────────────────────┤
│ 1 │ Safe    │ mainnet │ 0xSafe…  │ approved │ safeTxHash 0x…        │
│ 2 │ Safe    │ bsc     │ 0xSafe…  │ failed   │ unlock wallet: …      │
│ 3 │ Classic │ mainnet │ msig #7  │ approved │ confirm tx 0x…        │
└───┴─────────┴─────────┴──────────┴──────────┴───────────────────────┘

3 transaction(s): 2 ok · 1 failed
```

- `--continue-on-error` skips the `Continue with the remaining…?` question.
- `--confirm-once` (Safe refs only) reviews every signing card first, asks
  one `Sign all N reviewed Safe approval(s)?` question and then signs each
  without further prompts. Anything that broadcasts a transaction — on-chain
  `approveHash`, auto-execution when the threshold is met, Classic
  confirmations — still asks per item.
- Tx hashes without a network prefix use `-k/--network` (default: Ethereum
  mainnet), the same rule as `info` and the rest of jarvis. Prefixed hashes
  still win per item (`bsc 0x…` stays BSC).
- The process exits with status 1 when any item failed; skipped items alone
  keep it at 0.

## Directories

`jarvis wallet list`, `jarvis network list` and `jarvis msig chains list`
are bordered tables. `network list` prints RPC **hostnames** only so a
default Infura/Alchemy key never lands on the screen; full URLs live
under `jarvis node list <network>`.
