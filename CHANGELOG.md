# Changelog

## Unreleased — terminal output redesign

Everything jarvis prints was reworked so that the information you act on
comes first and reads the same way across `info`, signing and batch flows.
No RPC, signing or storage behaviour changed. `--json-output` files are
byte-for-byte compatible apart from the new `transfers`, `net_effect`,
`revert_reason`, `block_number` and per-log `address` fields.

### `jarvis info <hash>`

- Headline line: status glyph, method, destination; a muted second line
  carries from / value / gas / nonce / block.
- Function call shown as an indented tree; arrays longer than a few items and
  long `bytes` values collapse. Events are one line each.
- `--degen` expands collapsed values, shows full addresses and switches the
  parameter list back to a bordered table.
- Addresses are name-first (`me (0x9642…5D4E)`); unknown ones are bare
  short hex, the zero address is labelled `(zero address)` even if the
  address book has a name for it. Only an *unknown call target* is
  highlighted in yellow. Each tx ends with a one-line footer (status,
  method, hash) so the outcome is visible even after a long event list.
- **Net effect** block (per-address net token change) when a tx has four or
  more token movements. The per-hop Transfers list is not printed; Events
  already has every movement. `--json-output` still includes `transfers`.
- Integers carry thousands separators (`1,000,000,000`); token amounts are
  rounded to four decimals in compact views. `--degen`/JSON keep full
  precision.
- Event lines wrap to the terminal width instead of running off screen.
- Revert reasons are recovered by replaying the call (`Error(string)`,
  `Panic`, custom errors by name) and shown under the headline; JSON gains
  `revert_reason` and `net_effect`.
- `info` never gives up on a tx: unverified contracts, rate-limited
  explorers, contract creations and plain value transfers into contracts
  all render, with undecoded calls/events shown raw and counted.
- Overloaded methods show their Solidity name (`execute`, not `execute0`).
- The `Network:` header and the bare hash echo are gone; the network sits on
  the details line.
- Time-named integer parameters (`deadline`, `expiry`, `validUntil`,
  `unlockTime`, …) holding a plausible unix time show the date and how far
  away it is: `2026-09-07 03:30:00 UTC, in 30 min`. The compact view shows
  the date alone; `--degen` keeps the raw seconds. The same applies to the
  echo of a typed parameter, so a stale deadline shows as "… ago" before
  signing.
- When an ERC-7730 descriptor matches, a green **Clear Signed** panel is
  printed above the ABI call — the same fields as at sign time, including
  inner MultiSend / Safe destinations, but without the hardware-wallet
  comparison hint. No match leaves the output unchanged.

### Interactive parameter entry

- The "You entered" table after each parameter is replaced by one `→` line
  (rewritten in place on a TTY); complex values expand as a small tree.
- The method header now reads `method → contract`.

### Signing screen

- One signing card for EOA transactions, Safe proposals/approvals/executions
  and WalletConnect requests: decoded call first, then Sign with / Send to /
  Gas / nonce directly above the prompt.
- Derived warnings printed as `!` lines right before the prompt: destination
  not in the address book, native value into a contract, native value attached
  to an ERC-20 call, undecodable calldata, unlimited ERC-20 approval,
  `setApprovalForAll`, approval to an unknown spender, Safe `DELEGATECALL`.
- Native ETH sends to an EOA and ERC-20 `transfer` / `transferFrom` open as
  `Send  1.5 ETH  →  Alice` / `Send  1,000 USDC  →  Alice` rather than a
  generic Call header. Safe cards label that EOA destination `Recipient`.
- Safe cards list the Safe, operation, nonce, `safeTxHash` and collected
  signatures; the EOA card for a Safe execution collapses the inner call
  since it was just shown.
- Prompts say what will happen and what it costs, e.g.
  `Sign and broadcast (≈ 0.0017 ETH)?` /
  `Sign and submit this Safe proposal (off-chain, no gas)?`.
- `Sign with` shows the wallet's own name when the address book has none,
  plus the wallet kind (`ledger`, `trezor`, `keystore`) so a hardware prompt
  is expected. Addresses are always EIP-55 checksummed; the token decimal
  count no longer leaks into `Send to`.
- Gas reads cost-first: `≈ 0.0017 ETH   (85,123 gas × max 20 gwei, tip 1.5
  gwei)`.
- New warning when the signer's balance does not cover value + max gas.
- Declining the prompt prints `Cancelled — nothing was signed or sent.` and
  exits quietly instead of "Failed to proceed after signing".
- Gas-estimation failures are explained (insufficient funds with the
  wallet's balance, reverting call) instead of dumping every node's error.
- `send --to 0x…` accepts a literal address that is not in the address
  book; `send -g <limit>` no longer drops the amount. EIP-7702 delegated
  EOAs are not treated as contracts.

### Interactive parameter entry (continued)

- Integer parameters of an ERC-20 call echo with the token amount
  (`1000000 (1 USDC)`); a rejected answer is followed by the accepted
  input forms for that type. Integers accept `1e6`, `1_000_000` and
  `1,000,000`; a bare fraction is rejected with a hint to add the token.

### Waiting and results

- Broadcasting prints `✓ broadcast` followed by a live status line
  (not in mempool → in mempool → mined / reverted / dropped) with elapsed
  time; off-TTY each state is printed once.
- After mining, the post-sign view shows the headline, Net effect and Events
  (gas used / limit in the details line) instead of the full `info` dump.
- Waiting for a Ledger uses the same status line with a countdown.

### Batch approvals (`jarvis msig bapprove`)

- Plan printed before the first prompt; each item runs under a `[i/n]`
  banner with everything indented; one-line result with a running tally.
- After a failed item jarvis asks whether to continue; declining records the
  rest as skipped.
- Unified summary table for Safe and Classic items, totals line, and exit
  status 1 when any item failed.
- New flags:
  - `--continue-on-error` — never ask after a failure, keep going.
  - `--confirm-once` — Safe refs only: review every signing card first,
    confirm once, then sign all of them. Broadcasts (on-chain `approveHash`,
    threshold auto-execution, Classic confirmations) still confirm per item.

### Help and wallets

- `jarvis --help` is a one-screen overview: the five commands you reach for,
  a wrapped alphabetical list of networks with aliases in parentheses
  (`mainnet (ethereum)`, `matic (polygon)`), where nodes live, and each
  block-explorer API-key variable once. `-k/--network` lists every network
  once instead of aliases as separate entries.
- `jarvis wallet add` offers the wallet kinds as a numbered menu with their
  derivation path or what will be asked next, instead of asking you to type
  one of five words. Hardware-wallet address paging uses the same numbered
  menu (`next page` / `previous page` / `custom derivation path`) instead of
  typed keywords mixed with 0-based indices.
- `jarvis wallet list` is a table (Address / Kind / Description);
  `jarvis addr` / `whois` drop the dashed rule and the `(Unknown)` label;
  `jarvis network list` is a table of names, chain IDs and RPC **hosts**
  (never the full URL, so default Infura keys stay off the screen);
  `jarvis msig chains list` uses the same table primitive.
- Classic `msig info` / `approve` use the same signing card as Safe (decoded
  call, Multisig / Tx ID / Status / Signed by) instead of the old bordered
  box; the following EOA confirm/revoke/execute card collapses the inner
  call. Classic `msig summary` lists only the pending queue (id, destination,
  value, sigs, status) rather than every historical tx id. Confirmation and
  execution log scans use the same `Spinner` as other waits instead of a
  raw `\r` percentage.
- `jarvis version` leads with `jarvis <version>` and the GitHub URL, then
  the existing Kyber contact lines.
- `--from` help points at `jarvis wallet list` (there is no `jarvis acc`);
  `--gasprice` no longer mentions ethgasstation.info; `--abi` fetches from
  the block explorer (not “etherscan”); `--amount` is native-token units,
  not hard-coded ETH. `info --json-output` describes this command;
  `-x/--degen` says what it expands.
- `msig new` picks Safe vs Classic with the same numbered menu as
  `wallet add`. Classic `msig gov` uses a `Section` header and
  “On-chain txs” (the old “transaction inited” line is gone).
- WalletConnect Classic/Safe `eth_sendTransaction` uses the same signing
  card as `msig init` (inner Classic call + collapsed EOA wrap, or Safe
  proposal card). Compact colon-block confirms remain only as a fallback
  when the UI is not a full terminal, and for `personal_sign` / typed data.
- `contract encode --help` is a short parameter summary instead of a
  50-line spec; `contract read` output uses the same param tree as a call
  body, not a bordered table per return value.
- Signing warnings and inner-call `value` labels use the network's native
  decimals/symbol (not hardcoded 18 / ETH). Safe cards drop the raw-wei
  parenthetical. Cancelling any confirm prints
  `Cancelled — nothing was signed or sent.`
- `send --from <safe>` no longer reprints Safe address/version/threshold
  before the signing card (those are on the card). Classic `msig init`
  gas-estimation failures use the same explained error as `send`.

### Internal

- `ui.UI` gained `SeverityMuted`, `Subsection`, `RewriteLastLine`,
  `KeyValueCells` and a `Progress` handle returned by `Spinner`;
  `TableWithGroups` was removed (use `PrintTable` with `Table.Groups`).
- `Subsection` no longer stacks a second blank line on top of one that is
  already there.
- Transaction rendering is split into a `TxDisplay` view-model
  (`util/display_model.go`) and layout-driven renderers
  (`util/display_tx.go`, `LayoutInfo` / `LayoutInfoFull` / `LayoutPostSign`).
