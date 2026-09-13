# How `vet` / `--careful` would have treated documented EVM phishing cases

Evaluated 2026-09-13 on branch `vet` (`4afb8ad` plus this report) against the attached dataset of public hack/phishing incidents.

This is a **protection report**, not a claim that Jarvis would have been in the victim's wallet at the time. Each case is replayed as the **victim-signed** transaction (or typed-data Permit) that granted the loss, using today's RPC + explorer view of those contracts.

## What was tested

Two product surfaces:

| Surface | What it runs | When |
|---|---|---|
| **`--careful` signing card** | `SigningWarnings` **plus** `vet.Analyze` / `AnalyzeTypedData` (`ModeFull`) | User is about to sign (EOA, Safe, Classic, WalletConnect) |
| **`jarvis vet`** | **Only** `vet.Analyze` / `AnalyzeTypedData` | Offline review; no signing-card lines |

Without `--careful`, signing cards keep the old always-on lines (unknown dest, ETH-into-contract, undecoded calldata, unlimited `approve` / `setApprovalForAll` / permit spender) plus **DELEGATECALL** from vet. None of these cases are Safe DELEGATECALLs.

Grok (`XAI_API_KEY`) was **unset** in this environment. Every verified-source path therefore ends with `AI review skipped: no AI client`. Findings below are **local measures only**.

Live checks used Kyber RPCs (Ethereum, BSC, Arbitrum) and explorer `GetVerifiedSource` / `HasCode`. `jarvis vet --data` / `--typed-data` was run on the same payloads.

**Do not treat operator cash-out or post-approve drain txs as victim interactions.** Those are marked out of scope.

## Scorecard

Verdicts assume a user who reads red/yellow `!` lines and refuses anything that contradicts the bait UI (e.g. "Send USDT" vs unlimited approve).

| Case | Category | `--careful` card | `jarvis vet` alone | Verdict |
|---|---|---|---|---|
| `usdt-999999-2026-07` | approval phishing | Unlimited USDT approve + unknown spender | AI skip only | **Likely stop** (card). Vet-alone misses the approve. |
| `trustwallet-qr-usdt-bsc-2026-05` | approval phishing | Unlimited BSC-USDT approve + unknown spender + proxy-impl danger | Proxy-impl danger only | **Likely stop** (card). Vet-alone does not say "unlimited approve". |
| `unibot-router-2023-10` | approved vulnerable router | Unlimited approve to new router + unknown spender | AI skip only | **Warn, depends on user.** Looks like a normal token approve to a "new official" router. |
| `okx-dex-proxy-compromise` | approved compromised protocol | Unlimited approve to **named** `TokenApprove` | AI skip only | **Would not stop.** Official spender, later key/proxy compromise. |
| `inferno-swap-bait-2023-08` | fake SWAP() | Unknown dest + ETH into contract + undecoded + unverified + unverified impl | Unverified + unverified impl | **Likely stop.** |
| `inferno-withdraw-operator-not-victim` | drainer cash-out | — | Unverified impl (noise) | **Out of scope** (operator, not victim). |
| `gmx-signal-transfer-arb-2023-11` | protocol-function phishing | Explorer name `GmxUnstakeCreator`; decoded `createAndCall`; **no local danger** | AI skip only | **Would not stop** without Grok. |
| `wbtc-1155-address-poison-2024-05` (intended in book) | address poisoning | **Poison danger** (2+2 match) | Same poison line | **Likely stop** if the real recipient was already saved. |
| same WBTC transfer, empty extra book | address poisoning | Nothing but AI skip | AI skip | **Would not stop.** |
| `usdt-50m-address-poison-2025-12` | address poisoning | Nothing but AI skip | AI skip | **Would not stop.** Intended dest unpublished; `_to` also misses `token_recipient`. |
| `aeth-4_2m-permit-create2-2024-01` | permit phishing | Unlimited typed-data Permit + unknown spender | Same | **Likely stop** if the Permit is signed through Jarvis. |
| `setapprovalforall-template` | NFT approval | Grants **ALL** tokens + unknown operator | AI skip only | **Likely stop** (card). Vet-alone misses it. |
| `flashusdt-fake-token-family` | fake token | Unlimited approve to explorer-named bot + impl `0x…14` | Impl warning | **Warn, weak.** Explorer names make the fake token look known. |
| `simulation-phishing-class-2024-2025` | simulation phishing | Same shape as Inferno bait (unverified + value + 4-byte) | Unverified | **Warn** on unverified/value. **Cannot** catch sim-vs-inclusion by itself. |

**Headline:** `--careful` is strong against **classic drainers** (unlimited approve / `setApprovalForAll` / unverified payable bait / Permit / poisoning when the real address is in the book). It is weak against **official-looking approves**, **Create2 protocol wrappers that explorers have named**, and **poisoning when the intended address was never saved**. `jarvis vet` without the signing card **does not mention unlimited approve or setApprovalForAll**.

---

## How to read a case

Victim interaction is almost never the later `multicall` / `withdraw` / `transferFrom`. Typical shapes:

1. **Approve the token**, spender = attacker (or a router that later gets exploited).
2. **Call a phishing `to`** with a 4-byte selector and maybe `msg.value`.
3. **Transfer the real token** to a lookalike `to` argument.
4. **Sign typed-data Permit**; attacker submits on-chain later.

Infrastructure in the dataset:

- Inferno receiver `0x000037bb…0000` — victims usually do **not** call this.
- Canonical Permit2 `0x000000000022D473030F116dDEE9F6B43aC78BA3` is **legitimate**. The mistake is the spender inside the signature.

---

## Cases

### 1. `usdt-999999-2026-07` — ~$1M USDT approval

**What the victim signed:** `approve` on official Ethereum USDT (`0xdAC1…1ec7`). Exact spender unpublished; stand-in is drain contract `0x1178…0502` (`Fake_Phishing4558420`). Unlimited `uint256`.

**`--careful`:**

- `approves UNLIMITED USDT token to 0x1178…0502`
- `spender 0x1178…0502 is not in your address book`

**`jarvis vet`:** destination is verified USDT → no `unverified`. Only `AI review skipped`.

**Protection:** The card tells the user this is not a send. That is the whole scam. Vet-alone would **not**.

---

### 2. `trustwallet-qr-usdt-bsc-2026-05` — fake “Send USDT” QR

**Exact calldata** (BSC-USDT `0x55d3…7955` → spender `0x76C7…a169`, max uint):

```
0x095ea7b300000000000000000000000076c77b171264e1a81baa63125d9627e70b43a169
ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff
```

**`--careful`:** unlimited `BEP20USDT` approve + unknown spender. Vet also raised **danger** `proxy_impl_unverified` on official BSC-USDT → impl `0xF68a…F62d`.

**`jarvis vet`:** only the proxy-impl line. Confirmed on CLI.

**Protection:** Card vs bait UI is decisive. The red proxy-impl line is about the **official token proxy**, so it would also fire on a real BSC-USDT approve (warning fatigue).

---

### 3. `unibot-router-2023-10` — approve a vulnerable “official” router

**Victim path:** unlimited approve of a token (replayed as USDT) to router `0x126c9Fba…1865`.

**`--careful`:** unlimited approve + spender not in book.

**Attacker path (not victim):** selector `0xb2bd16ab` on that router. Today the router is still unverified and reports an unverified implementation. Victims did not sign this.

**Protection:** Same as any unlimited approve to a new contract. Users who believed Unibot’s announcement would still click through. Grok on verified USDT might have commented on the spender; not tested.

---

### 4. `okx-dex-proxy-compromise` — official OKX contracts, later upgrade

**Victim path:** unlimited approve of USDT to official `TokenApprove` `0x40aa…cd7f`.

**`--careful`:**

- `approves UNLIMITED USDT token to 0x40aA…Cd7f (TokenApprove)`
- Spender **is** treated as known (explorer/book name). No “unknown spender”.

**Protection:** **Would not stop.** This is indistinguishable from a normal DEX approve. The loss was a later proxy-admin key compromise. Vet does not time-travel. A later `upgradeTo` **by the attacker** would be a danger `upgrade` finding if someone vetted *that* tx — users never signed it.

---

### 5. `inferno-swap-bait-2023-08` — fake `SWAP()`

**Exact victim tx** `0x9f042b…`: `to` `0x000056c3…0000`, data `0x04d84108`, value `8192450711968105` wei.

**`--careful`:**

- dest not in address book
- sends `0.008192 ETH` into a contract
- calldata could not be decoded
- **danger** unverified source
- **danger** proxy → unverified impl `0x0000aF47…0000`

**`jarvis vet`:** both danger lines (CLI confirmed).

**Protection:** **Likely stop.** This is the class vet’s unverified + value + undecoded measures were built for.

---

### 6. `inferno-withdraw-operator-not-victim`

Exact `withdraw(uint256,address)` to `0x0000DAf6…0000`. Decoded. `withdraw` is **not** in the drain-method list (that list is `sweep` / `rescue*` / `emergencyWithdraw` / `skim` — so ordinary WETH `withdraw` stays quiet).

Vet still flags unverified implementation. **Out of scope** for user protection. Included so this calldata is not mistaken for a victim tx.

---

### 7. `gmx-signal-transfer-arb-2023-11` — the published tx is not `signalTransfer`

Dataset described `signalTransfer` on the GMX Reward Router. The published Arbitrum tx `0x0b8d095c…fb96` was fetched live.

| Field | Value |
|---|---|
| `to` | `0x00004d035111f89dC358edc8D316FD2Ad3F80000` |
| Explorer / Jarvis name | `GmxUnstakeCreator` |
| Selector | `0xd8abd326` |
| Decoded method | `createAndCall` |
| Notable args | `victim`, `percentageForFirstAddressInBasisPoints = 10000` (100%), two unknown payout addresses |

**`--careful`:** because the explorer named the dest, `SigningWarnings` did **not** say “not in your address book”. Calldata decoded, so no undecoded warning. `createAndCall` is not `create` (that flag is only for a nil-`to` deploy), not admin/upgrade/drain, not a token `transfer`. Result: **AI skip only**.

**`jarvis vet`:** same (CLI confirmed).

**Protection:** **Would not stop** on local measures. This is a Create2 factory wrapping a protocol action. Grok on verified source is the only v1 hook that might have said “this sends 100% of the position to an unknown address” — and it did not run here.

`signalTransfer(address)` on the real Reward Router would also miss: that name is not in the method lists, and the receiver is not scored as `token_recipient`.

---

### 8. `wbtc-1155-address-poison-2024-05` — 1,155 WBTC

Lookalike `0xd9A1C378…3a91` vs intended `0xd9A1b0B1…3a91` (first 2 bytes + last 2 bytes match). Transfer of `115500000000` raw units on real WBTC.

**Intended address in the book:**

```
! 0xd9A1C378…3a91 looks like your address-book entry 0xd9A1b0B1…3a91 (intended WBTC recipient)
```

Danger `poison`. **Likely stop.**

**Empty extra book (bundled tokens only):** no poison pair. **Would not stop.**

`token_recipient` (“recipient not in your address book”) did **not** fire in either run. Standard ERC-20 ABI names the dest `_to`; the measure only looks for `to` / `recipient` / `dst`. That is a local-measure miss on the exact transfer shape of this loss.

---

### 9. `usdt-50m-address-poison-2025-12`

Transfer of real USDT to reported phisher `0xbaff2f13…f8b5`. Intended dest unpublished.

**Result:** AI skip only. No poison (nothing in the book 2+2-matches). No `token_recipient` (`_to` again). Dest is official USDT, so no unverified warning.

**Protection:** **Would not stop** unless the user had already saved the real counterparty (then poison might fire) or `_to` is treated as a recipient.

---

### 10. `aeth-4_2m-permit-create2-2024-01`

Off-chain EIP-2612 Permit. Replay: verifying contract `aEthWETH` `0x4d5F47FA…14E8`, spender `0x0000372B…F688`, value = max uint.

**`jarvis vet --typed-data` (CLI):**

```
! typed-data Permit: unlimited allowance to spender 0x0000372B…F688 on token 0x4d5F47FA…14E8
! typed-data spender 0x0000372B…F688 is not in your address book
```

**Protection:** **Likely stop** if Jarvis sees the typed-data (WalletConnect Permit path / `jarvis vet --typed-data` / `--careful` on that signature). Create2 freshness does not hide an unlimited Permit once the message is decoded. A wallet that only shows “Sign message” without routing through vet would still lose.

If the verifying contract itself is the fresh Create2 address, vet also adds danger `unverified`.

---

### 11. `setapprovalforall-template`

Representative `setApprovalForAll(operator, true)` on BAYC, operator = rotating drainer stand-in.

**`--careful`:** `grants … control over ALL BoredApeYachtClub tokens` + unknown spender.

**`jarvis vet`:** AI skip only. There is no vet measure for `setApprovalForAll`.

**Revoke contrast** (published `approved=false`): no “ALL tokens” line. Correct. That collection is unverified today, so vet still raises unverified — a **false-ish positive** on a benign revoke.

---

### 12. `flashusdt-fake-token-family`

Impostor `0x07d0aacd…f1dc` (not Tether `0xdAC1…1ec7`).

Unlimited approve: card says `approves UNLIMITED FlashUSDT` to explorer-named `FlashUSDTLiquidityBot`. Spender-not-in-book **did not** fire (name counts as known). Vet: `proxy_impl_unverified` → impl `0x0000…0014` (precompile 20; likely a junk EIP-1967 slot).

Bare `claim()` + 0.01 ETH: ETH-into-contract + undecoded + same impl warning.

**Protection:** Some warning, but explorer names make the fake token look like a real asset. Users told to “activate FlashUSDT” may ignore an unlimited-approve line that uses the same brand.

---

### 13. `simulation-phishing-class-2024-2025`

Class: payable `claim()` (e.g. `0x4e71d92d`) on a fresh unverified contract. Replay used the Inferno bait address (same shape: 4-byte + value + no source).

**`--careful`:** unknown dest, ETH into contract, undecoded, unverified, unverified impl.

**Gap:** v1 has **no** `eth_call` / balance-diff simulation. It cannot detect “sim shows a tiny credit, inclusion drains you.” Protection is the unverified + value heuristic, which is the right first filter for this class.

---

## Cross-cutting gaps (local measures, no Grok)

1. **`jarvis vet` omits SigningWarnings.** Unlimited `approve` / `increaseAllowance` / `setApprovalForAll` only appear on the signing card. Reviewing with `jarvis vet --data` can look “clean” on official USDT.
2. **No Grok in this run.** GMX `createAndCall` and “is this OKX spender still safe?” are the cases that needed source review.
3. **Official spender / later compromise (OKX, Unibot-as-announced).** Unlimited approve to a *named* protocol contract is treated as a normal DEX approve.
4. **Explorer names count as known.** `GmxUnstakeCreator`, `TokenApprove`, `FlashUSDT` suppress “not in your address book.”
5. **`token_recipient` misses `_to` / `_dst`.** Standard ERC-20 ABIs use `_to`. Poisoning transfers to an unknown lookalike produce **no** recipient warning unless a book entry 2+2-matches.
6. **Poison needs the real address in the book.** Empty book → silent WBTC / USDT sends.
7. **No factory / `createAndCall` measure.** Nil-`to` create is covered; Create2 “create and call the protocol” is not.
8. **No protocol-function list** (`signalTransfer`, reward assignment, …).
9. **No simulation / state-diff.** Simulation phishing is only caught when the dest is unverified or sends ETH.
10. **`proxy_impl_unverified` false-ish positives.** Official BSC-USDT; junk impl `0x…14` on FlashUSDT. Red danger on legitimate proxies trains users to ignore red.
11. **Drain list does not include `withdraw`.** Intentional for WETH; Inferno operator `withdraw` would not show as drain anyway (out of scope).

---

## What already works well

- **Unlimited ERC-20 approve** and **NFT `setApprovalForAll(true)`** on the `--careful` card, including unknown spender.
- **Unverified payable bait** (Inferno `SWAP()`, simulation-class `claim()`): unknown dest + ETH-into-contract + undecoded + danger unverified.
- **Typed-data unlimited Permit** + unknown spender (`jarvis vet --typed-data` and the WC Permit path).
- **2+2 poisoning** when the intended address is in the address book (WBTC lookalike reproduced exactly).
- **Revoke vs grant:** `setApprovalForAll(..., false)` does not claim “ALL tokens”.
- Operator Inferno `withdraw` is easy to keep out of the victim column.

---

## Practical guidance

Use **`--careful` at sign time**, not only `jarvis vet`, for approve / NFT / send flows.

Save frequent counterparties (CEX deposit, own cold wallet) in the address book. Poisoning protection is that lookup.

Treat unlimited approve to a *named* protocol as a real risk (OKX/Unibot class). Vet will not refuse it.

Do not vet attacker `multicall` / `withdraw` txs and call that user protection.

Grok still needs `XAI_API_KEY` before claiming coverage of verified-but-hostile factories (`createAndCall`, 100% `percentageForFirstAddressInBasisPoints`).

---

## How this was run

- Dataset: attached `evm_hack_phishing_cases_998d.json` (13 cases + Inferno/Permit2 infra).
- Harness called the same `SigningWarnings` + `vet.Analyze` / `AnalyzeTypedData` as `--careful` / `jarvis vet`, with `ExplorerLookup` on live networks.
- CLI spot-checks: Inferno `SWAP()`, Trust Wallet BSC approve, WBTC lookalike transfer, USDT unlimited approve, GMX published `createAndCall`, aEthWETH Permit, Inferno operator `withdraw`.
- No private keys, no broadcasts, no address-book writes committed.
