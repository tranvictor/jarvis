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
- New **Transfers** section derived from `Transfer` and `Approval` logs, with
  `UNLIMITED` marking max-uint approvals.
- Function call shown as an indented tree; arrays longer than a few items and
  long `bytes` values collapse. Events are one line each.
- `--degen` expands collapsed values, shows full addresses and switches the
  parameter list back to a bordered table.
- Addresses are name-first (`me (0x9642…5D4E)`); unknown ones are bare
  short hex, the zero address is labelled. Only an *unknown call target* is
  highlighted in yellow. Each tx ends with a one-line footer (status,
  method, hash) so the outcome is visible even after a long event list.
- **Net effect** block (per-address net token change) when a tx has four or
  more transfers; the transfer list is capped at eight in the compact view.
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
- Transfer rows are laid out in columns (`amount   from  →  to`) so arrows
  and destinations line up; approvals read `from  approves  spender` with
  `UNLIMITED <token>` in the amount column, deposits/withdrawals name the
  mechanism in place of the missing party.
- Time-named integer parameters (`deadline`, `expiry`, `validUntil`,
  `unlockTime`, …) holding a plausible unix time show the date and how far
  away it is: `2026-09-07 03:30:00 UTC, in 30 min`. The compact view shows
  the date alone; `--degen` keeps the raw seconds. The same applies to the
  echo of a typed parameter, so a stale deadline shows as "… ago" before
  signing.

### Interactive parameter entry

- The "You entered" table after each parameter is replaced by one `→` line
  (rewritten in place on a TTY); complex values expand as a small tree.
- The method header now reads `method → contract`.

### Signing screen

- One signing card for EOA transactions, Safe proposals/approvals/executions
  and WalletConnect requests: decoded call first, then Sign with / Send to /
  Gas / nonce directly above the prompt.
- Derived warnings printed as `!` lines right before the prompt: destination
  not in the address book, native value into a contract, undecodable
  calldata, unlimited ERC-20 approval, `setApprovalForAll`, approval to an
  unknown spender, Safe `DELEGATECALL`.
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
- After mining, the post-sign view shows the headline, Transfers and Events
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
  one of five words.

### Internal

- `ui.UI` gained `SeverityMuted`, `Subsection`, `RewriteLastLine`,
  `KeyValueCells` and a `Progress` handle returned by `Spinner`;
  `TableWithGroups` was removed (use `PrintTable` with `Table.Groups`).
- `Subsection` no longer stacks a second blank line on top of one that is
  already there.
- Transaction rendering is split into a `TxDisplay` view-model
  (`util/display_model.go`) and layout-driven renderers
  (`util/display_tx.go`, `LayoutInfo` / `LayoutInfoFull` / `LayoutPostSign`).
