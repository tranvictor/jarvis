# Improved terminal UI — implementation plan

Branch: `improved-ui`. Each phase below is meant to land as its own PR into
`improved-ui`; the branch merges to `master` once Phase 5 is done.

## Goals

1. A user reading `jarvis info <hash>` finds status, intent and asset movement
   in the first screen, without scrolling through bordered tables.
2. A user signing a tx sees everything that matters in the last ~8 lines above
   the `[Y/n]` prompt, with derived risk warnings, and never sees a truncated
   address on a signing screen.
3. Waits (mining, hardware wallet) show liveness.
4. Batches show a plan, a running tally, and end with a scannable summary and a
   meaningful exit code.
5. Nothing changes for piped / `-Y` / `--json-output` consumers except additive
   fields.

## Non-goals

- Full-screen TUI (bubbletea). The transcript model stays.
- Pager integration.
- Truncating raw calldata or addresses anywhere on the signing path.
- New config file options. Behaviour is driven by existing flags (`-x`, `-Y`,
  `--json-output`) plus at most two new batch flags (Phase 5).

## Design rules (apply to every phase)

- Hierarchy through weight, not colour: values are normal, metadata is dim.
  Colour is reserved for meaning: green = success/known signer, yellow = needs
  attention, red = failure, bold = must read before an irreversible action.
- Yellow is scarce. Unknown addresses in params/logs are dim, not yellow.
  Yellow is for: unknown `to` of a call, value sent into a contract, unlimited
  approvals, `DELEGATECALL`, ERC-7730 mismatch, and revert/lost warnings.
- Bordered tables only for truly tabular data (3+ columns). Key/value blocks
  and tree glyphs (`├─`, `└─`, `↳`) for everything else.
- Every interactive screen ends with its most important line directly above
  the prompt. Every command ends with a one-line result.
- TTY-only features (spinner, cursor rewrite) are gated on
  `term.IsTerminal(stdout)` exactly like colours are today. Off-TTY, print one
  plain line per state change.
- The view-model (`TxDisplay`, `FunctionCallDisplay`, ...) stays the single
  source for both terminal and JSON. New terminal content means new view-model
  fields, never string formatting inside `cmd/`.
- All output is testable through `ui.RecordingUI`; each phase adds
  transcript-style tests that assert the exact lines.

---

## Phase 0 — UI primitives

Foundation the later phases rely on. No user-visible behaviour change on its
own except the styling of `Section` and the status cell.

Files: `ui/ui.go`, `ui/terminal.go`, `ui/recording.go`, `ui/table.go`,
`common/printer.go`.

- `SeverityMuted` added to `Severity`; `TerminalUI.Style` renders it with
  aurora `Faint`. `RecordingUI` returns plain text as today.
- `Section`: bold title, dim rule. Add `Subsection(title)` (a bold line with no
  rule) for the "Transfers" / "Events (n)" headings.
- `TerminalUI.Spinner` becomes a status spinner: returns `update(msg string)`
  and `stop(final StyledText)` so callers can change the message while it runs
  and replace it with a final line. Elapsed time is appended automatically.
  Off-TTY: `update` prints a plain line only when the message changes.
- `KeyValue` gets a styled variant `KeyValueCells(rows [][2]TableCell)` so
  labels can be dim and values coloured. Existing `KeyValue` delegates to it.
- `RewriteLastLine(s string)` on `UI` (no-op in `RecordingUI` and off-TTY):
  cursor-up + clear + write. Used by Phase 2 only; keep it small.
- Address formatting helpers in `common/printer.go`:
  `ShortAddress(a) string` (`0x9642…5D4E`) and
  `NameFirst(a Address, full bool) string`
  (`Uniswap V2 Router (0x7a25…488D)` or with the full hex). Unknown addresses
  render hex-first with a dim `(unknown)`.
- Tree row helper in `util/display.go`: `treePrefix(depth int, last bool)`
  producing `│  `, `├─ `, `└─ ` so nested params/inner calls share one style.

Tests: `ui/terminal_test.go` for muted style and spinner off-TTY behaviour;
`common/printer_test.go` for the address helpers.

---

## Phase 1 — `jarvis info` layout

Files: `util/display_model.go`, `util/display.go`, `util/util.go`,
`common/ethereum_data.go`, `txanalyzer/analyzer.go`, `cmd/info.go`,
`util/erc20_util.go`.

### Data

- `TxResult` gains `BlockNumber` and `RevertReason` (from the receipt / an
  `eth_call` replay when status is reverted; best-effort, empty if unavailable).
- `TxDisplay` gains:
  - `Headline HeadlineDisplay{Status, Method, To, From, Value, GasCost, Nonce,
    Block, RevertReason}`
  - `Transfers []TransferDisplay{Token, Amount, From, To, Kind}` where `Kind` is
    `transfer | approval | deposit | withdrawal`, derived from logs whose
    signature matches ERC-20/ERC-721 `Transfer`, `Approval`, WETH-style
    `Deposit`/`Withdrawal`. Amounts use existing decimals lookup; unlimited
    approvals keep the `∞` label and are flagged.
- `DisplayTxResult(..., fullDetail bool, ...)` becomes
  `DisplayTxResult(..., layout TxLayout, ...)` with
  `LayoutInfo`, `LayoutInfoFull` (`-x`), `LayoutPostSign` (used in Phase 4).
  All callers updated; `AnalyzeAndPrint` takes the layout.

### Rendering (`printTxDisplay`)

Order for `LayoutInfo`:

1. Headline (bold): `✓ done   method  →  Name (0x…)` then a dim second line
   with from / value / gas / nonce. Reverted: `✗ reverted  "reason"` in red.
2. `Transfers` subsection, one line per movement (omitted if none).
3. `Call  method` subsection: key/value block for params, tree for
   tuples/arrays and inner calls. Collapsed: arrays/tuples show `[n items]`,
   bytes longer than 32 show `0x1234…abcd (n bytes)`.
4. `Events (n)` subsection: one line per event
   (`1. Transfer  USDC  from … to … value …`); unknown events print
   `topic0` shortened.
5. Headline repeated (single line) as footer; this also replaces the dashed
   separator between multiple hashes in `cmd/info.go`.

`LayoutInfoFull` (`-x`): same order, nothing collapsed, gas block in the key/
value card, events as today's 3-column table, full addresses everywhere.

Yellow policy change per design rules; `StyledAddress` gets a context
parameter (`ctxCallTarget | ctxParam`) so only call targets go yellow.

### JSON

`--json-output` shape is additive: `headline`, `transfers`, `blockNumber`,
`revertReason` are new keys; existing keys unchanged.

### Tests

`util/display_test.go`: transcript tests for a plain transfer, an ERC-20
transfer with two `Transfer` logs, a reverted call, a nested multicall, and
`-x`. Assert exact lines via `RecordingUI`.

---

## Phase 2 — Parameter entry

Files: `cmd/util/prompt_util.go`, `cmd/util/prompt_util_test.go`,
`util/display.go`.

- `PromptFunctionCallData` prints the method header once:
  `method  →  Name (0x…)` and then each param as
  `N. name  type` (type dim) followed by the `> ` prompt.
- On accepted input, replace `You entered:` + table with `Interpret` lines:
  scalar → one `→ value` line; array/tuple → `→ [n]` then tree rows. On a TTY,
  `RewriteLastLine` folds the label + answer into a single line so the
  transcript is a compact form; off-TTY the lines stay as printed.
- Invalid input prints `✗ reason` (red) under the prompt and re-asks; no table.
- `DisplayParam` stays for other callers but is no longer used here.

Tests: transcript for address + uint + address[] entry, including one invalid
attempt, in `RecordingUI` (off-TTY path).

---

## Phase 3 — Signing card

Files: new `cmd/util/signing_card.go` (+ test), `cmd/util/prompt_util.go`,
`cmd/safe_display.go`, `cmd/safe.go`, `cmd/safe_batch.go`, `cmd/util/util.go`.

### View-model

```go
type SigningCard struct {
    Kind      string            // "EOA transaction" | "Safe proposal" | "Safe approval" | "Safe execution"
    Network   string
    Signer    ui.StyledText
    To        ui.StyledText     // or "create contract at 0x…"
    Value     string
    Gas       string            // "max 20.0 gwei, tip 1.5 gwei · 85,123 gas · ≈ 0.0017 ETH"
    Nonce     string
    Safe      *SafeFields       // Operation, SafeNonce, SafeTxHash, Signatures, Executes (link to approved hash)
    ClearSign *erc7730.Rendered // existing ERC-7730 output, optional
    Call      *util.FunctionCallDisplay
    Warnings  []ui.StyledText   // yellow "!" lines
    Prompt    string            // "Sign and broadcast (≈ 0.0017 ETH gas)?"
}
```

### Rendering order

1. `Section(Kind)`
2. ERC-7730 box (if any)
3. Function call, full detail, full addresses, tree for nested calls
4. Key/value summary: Sign with / Send to / Value / Gas / Nonce (+ Safe fields)
5. Warnings, one per line, yellow `!` prefix
6. Blank line, then `Confirm(card.Prompt)` unless `-Y`

### Warnings (derived in one function, unit-tested)

- `to` not in address book
- value > 0 and `to` is a contract
- any `approve`/`setApprovalForAll`/`increaseAllowance` with unlimited amount
  or to an unknown spender
- `DELEGATECALL` operation (Safe)
- ERC-7730 descriptor found but `to` differs from the descriptor contract
- calldata present but ABI could not be resolved (undecoded call)

### Wiring

- `showTxInfoToConfirm` builds a card and calls `ShowSigningCard`; same for
  `showSafeTxToConfirm*`. Both keep their signatures so callers do not change.
- Safe approve → execute path: the execution card sets `Safe.Executes` to the
  safeTxHash just approved and renders inner calls collapsed (one line each)
  with a dim `shown above` note.
- Prompts: `Sign and broadcast (≈ X ETH gas)?`,
  `Sign approval (off-chain, no gas)?`, `Broadcast approveHash (≈ X ETH gas)?`,
  `Sign and submit this Safe proposal?`.

Tests: transcript tests for EOA transfer, EOA contract call with an unknown
`to`, Safe MultiSend proposal with `DELEGATECALL`, and Safe execution linked to
an approval.

---

## Phase 4 — Waits and post-sign delta

Files: `util/util.go` (`DisplayWaitAnalyze`, `BlockingWait`),
`cmd/util/util.go` (`HandlePostSign`), `accounts/*` (hardware wallet waits).

- `DisplayWaitAnalyze` uses the Phase 0 spinner:
  `waiting for 0xabc…def · in mempool` → `mined  block N  after M:SS  gas used
  a / b (p%)` or `✗ reverted …` or `✗ not seen for 3 min — tx may be lost;
  rerun with --retry-broadcast` (red).
- Post-mine rendering uses `LayoutPostSign`: headline, transfers, events; the
  function call is printed only when the tx reverted (with the revert reason).
- `BROADCASTED TX:` stays bold and remains the first line after signing.
- Hardware wallet / keystore waits: replace the periodic "still waiting"
  `Info` lines with the spinner (`Confirm on Ledger…`). `accounts` currently
  prints directly; thread a `ui.UI` through `UnlockAccount` or accept a
  minimal `Progress` interface to avoid an import cycle.

Tests: `util/util_test.go` with a fake reader that returns pending → mined;
assert off-TTY state lines.

---

## Phase 5 — Batch flows

Files: `cmd/msig.go` (classic `bapprove`), `cmd/safe_batch.go`,
`cmd/info.go` (multi-hash), `cmd/root.go` (flags).

- Plan step: before item 1, print one line per item
  (`[1/10] mainnet · Safe 0xSafe… · nonce 17 · multiSend (3 calls)`), then a
  single `Proceed with N items?` gate (skipped with `-Y`).
- Everything printed while processing item *i* is indented one level under the
  `[i/n]` banner; the banner itself stays bold.
- After each item, print a one-line result that stays in the transcript:
  `✓ [3/10] approved  safeTxHash 0x…`, `✓ [3/10] approved + executed  tx 0x…`,
  `– [4/10] skipped  already signed`, `✗ [5/10] failed  RPC rejected: …`.
- On failure, ask `Continue with the remaining N? [Y/n]` unless `-Y` or the new
  `--continue-on-error` flag is set.
- New `--confirm-once` for Safe approvals only: show all signing cards up
  front, one confirm, then sign each without further prompts. EOA broadcasts
  (classic msig confirms, Safe executions) always confirm per item.
- `printBatchSummary` / `printSafeBatchSummary` render a table
  (`# | network | target | result | hash`) via `PrintTable`, result cell
  coloured. `Total:` line stays. JSON summaries unchanged.
- Exit code 1 if any item failed (not for skipped).
- `jarvis info a b c`: the Phase 1 headline footer replaces the dashed line.

Tests: existing batch tests extended with transcript assertions for a
3-item run containing one skip and one failure.

---

## Phase 6 — Cleanup and docs

- Remove `ui.Table`/`TableWithGroups` if no callers remain; fix the stale
  `PrintVerboseParamResultToWriter` reference in `ui.UI.Writer` docs.
- README: replace the output examples with the new layouts (`info`, confirm
  screen, batch summary).
- Release notes entry listing the visible changes and the two new flags.

---

## Target output (spec for transcript tests)

### `jarvis info <hash>`

```
✓ done   swapExactTokensForTokens  →  Uniswap V2 Router (0x7a25…488D)
         from 0x9642…5D4E (me)   value 0 ETH   gas 0.00213 ETH   nonce 412   block 19,234,567

Transfers
  1,000 USDC      0x9642…5D4E (me)             →  USDC/WETH pair (0x0d4a…c11c)
  0.3121 WETH     USDC/WETH pair (0x0d4a…c11c) →  0x9642…5D4E (me)

Call  swapExactTokensForTokens
  amountIn       1,000,000,000              uint256
  amountOutMin   311,200,000,000,000,000    uint256
  path           [2]                        address[]
  ├─ USDC (0xA0b8…eB48)
  └─ WETH (0xC02a…6Cc2)
  to             0x9642…5D4E (me)
  deadline       1,725,600,000

Events (4)
  1. Transfer  USDC   from 0x9642…5D4E  to 0x0d4a…c11c  value 1,000,000,000
  2. Sync      pair   reserve0 …  reserve1 …
  3. Transfer  WETH   from 0x0d4a…c11c  to 0x9642…5D4E  value 312,100,000,000,000,000
  4. Swap      pair   …

✓ done   swapExactTokensForTokens  →  Uniswap V2 Router   0x3f9a…e1c2
```

### Parameter entry

```
doStuff  →  SomeContract (0x1234…abcd)
  1. to        address     →  Vitalik Buterin (0xd8dA…6045)
  2. amount    uint256     →  1,500,000,000,000,000,000  (1.5 ETH)
  3. spenders  address[]   →  [2]
                              ├─ alice (0xaaaa…1111)
                              └─ 0xbbbb…2222 (unknown)
```

### Signing card

```
===== EOA transaction =====

Call  doStuff
  to        address    0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045 (Vitalik Buterin)
  amount    uint256    1,500,000,000,000,000,000 (1.5 ETH)
  spenders  address[]  [2]
  ├─ 0xaaaa…(full) (alice)
  └─ 0xbbbb…(full) (unknown)

Sign with   my-wallet (0xFrom…full)                              mainnet
Send to     SomeContract (0xContract…full)
Value       0 ETH
Gas         max 20.0 gwei, tip 1.5 gwei · 85,123 gas · ≈ 0.0017 ETH   nonce 42

! spenders[1] 0xbbbb…2222 is not in your address book

Sign and broadcast (≈ 0.0017 ETH gas)? [Y/n]
>
```

### Wait and post-sign

```
BROADCASTED TX: mainnet:0xabc…def
✓ mined  block 19,234,567  after 0:19  gas used 71,203 / 85,123 (84%)

Transfers
  …
Events (2)
  …
✓ done   doStuff  →  SomeContract   0xabc…def
```

### Batch

```
===== Batch Approve (Safe): 3 transactions =====
  [1/3] mainnet · Safe 0xSafe…1111 · nonce 17 · multiSend (3 calls)
  [2/3] mainnet · Safe 0xSafe…1111 · nonce 18 · transfer
  [3/3] arbitrum · Safe 0xSafe…2222 · nonce 4  · execTransaction
Proceed with 3 items? [Y/n]
>

━━━ [1/3] mainnet · Safe 0xSafe…1111 · nonce 17 ━━━
  (signing card)
  Sign approval (off-chain, no gas)? [Y/n]
  >
✓ [1/3] approved  safeTxHash 0x…

━━━ [2/3] … ━━━
– [2/3] skipped  already signed off-chain

━━━ [3/3] … ━━━
✗ [3/3] failed  RPC rejected: nonce too low
Continue with the remaining 0? (n/a)

===== Batch Approve Summary =====
 # │ network  │ target          │ result               │ hash
 1 │ mainnet  │ Safe 0xSafe…111 │ approved             │ 0x…
 2 │ mainnet  │ Safe 0xSafe…111 │ skipped              │ –
 3 │ arbitrum │ Safe 0xSafe…222 │ failed: nonce too low│ –
Total: 3 transactions (1 approved, 1 skipped, 1 failed)
```

---

## Risks and mitigations

- **Signing-screen regressions are the only dangerous kind.** Phase 3 keeps
  full addresses and full calldata; the warnings function is pure and
  unit-tested; transcript tests pin the exact output.
- **Decimals lookups for Transfers add RPC calls to `info`.** Use the existing
  ERC-20 cache; fall back to raw amounts with a dim `(raw)` marker on failure.
- **Callers of `DisplayTxResult` / `AnalyzeAndPrint`.** The `bool → TxLayout`
  change is a compile-time break, so nothing is missed.
- **Off-TTY behaviour.** Every TTY feature has an explicit off-TTY branch and a
  `RecordingUI` test, so CI and piped usage never see cursor codes.
