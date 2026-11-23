// Package portfolio builds a one-wallet book from the Kyber Data API and
// the RPC nodes jarvis already has.
//
// Ways this can fail, and what the code does about each:
//
//   - Missing API key: the command stops before any call.
//   - Gateway 401, 404, 429, 503 or a non-zero envelope code: balances are
//     required, so the command stops and prints the status. A 429 waits
//     for Retry-After once.
//   - A cursor that repeats, or a book longer than 30 pages: pagination
//     stops. A short page is preferred to a loop.
//   - Token batch or market batch fails: the book still prints. Marks fall
//     back to the catalog price on the balance row, and a warning says so.
//     Unknown tokens are simply absent from the batch result.
//   - A catalog price of 0, a null settlement, or a missing sell side is
//     not a price. The row stays unpriced and out of the total.
//   - Buy and sell differ. The mark is the sell side. Settlement is next.
//     The balance row's price is last and labeled indicative.
//   - No RPC node for a chain, a multicall miss, or a balanceOf revert:
//     that holding keeps the Kyber amount. The header names those chains.
//   - A chain read of zero replaces a stale Kyber amount.
//   - Rebasing tokens can move without a transfer. That is a warning, and
//     the amount is still the chain read.
//   - Exit tier is asked only for a holding at or above the exit minimum,
//     quoted into the chain native. The native holding itself has no tier.
//     A failed quote leaves the cell blank.
//   - The socket sends no resume cursor. A dead connection (45s of silence,
//     a dropped heartbeat, a close) reconnects with backoff and subscribes
//     again. An event is applied only when it is newer for that stream.
//     While a pipeline is degraded, that kind is ignored and a REST
//     snapshot is read.
//   - More than 200 streams: both kinds stay on the largest holdings and
//     the rest wait for the REST refresh.
//   - --watch on a pipe: the command stops. A snapshot is the transcript.
//   - JSON carries signed percents and full addresses, and no glyphs.
package holdings
