#!/usr/bin/env bash
# End-to-end test of `jarvis btc` against a real bitcoind regtest node.
#
# Usage: scripts/e2e-btc.sh
#
# Downloads Bitcoin Core (checksum pinned) into $CACHE unless BITCOIND points
# at an existing binary, then runs btc/e2e with the e2e build tag. Every
# transaction jarvis broadcasts is validated by bitcoind's consensus and
# relay-policy rules, so a signature or fee bug fails the run.
#
# Hardware scenarios run only when their emulators are configured:
#   Trezor:  TREZOR_FIRMWARE_DIR (trezor-firmware checkout with the emulator
#            built: xtask build firmware --emulator -m t2t1 -p test) and
#            TREZOR_PYTHON (that checkout's venv python)
#   Ledger:  SPECULOS_BIN and LEDGER_APP_ELF (the Bitcoin Test app built for
#            nanos2 and run in Speculos); optionally SPECULOS_API_LEVEL
# Set E2E_REQUIRE_TREZOR=1 / E2E_REQUIRE_LEDGER=1 to fail instead of skipping.
# See docs/bitcoin.md for how the emulators were built.
#
# The report (markdown and JSON, with every command and its output) is
# written to $ARTIFACTS (default ./btc/e2e/artifacts).
set -euo pipefail

VERSION=28.0
SHA256=7fe294b02b25b51acb8e8e0a0eb5af6bbafa7cd0c5b0e5fcbb61263104a82fbc
URL=https://bitcoincore.org/bin/bitcoin-core-$VERSION/bitcoin-$VERSION-x86_64-linux-gnu.tar.gz
CACHE=${CACHE:-$HOME/.cache/jarvis-e2e}
ROOT=$(cd "$(dirname "$0")/.." && pwd)

if [[ -z "${BITCOIND:-}" ]]; then
	if [[ "$(uname -sm)" != "Linux x86_64" ]]; then
		echo "this script downloads a Linux x86_64 bitcoind; set BITCOIND=/path/to/bitcoind on other platforms" >&2
		exit 2
	fi
	BITCOIND=$CACHE/bitcoin-$VERSION/bin/bitcoind
	if [[ ! -x "$BITCOIND" ]]; then
		mkdir -p "$CACHE"
		tmp=$(mktemp)
		trap 'rm -f "$tmp"' EXIT
		curl -fsSL "$URL" -o "$tmp"
		echo "$SHA256  $tmp" | sha256sum -c - >/dev/null || {
			echo "checksum mismatch for $URL" >&2
			exit 1
		}
		tar -xzf "$tmp" -C "$CACHE"
	fi
fi
export BITCOIND
export E2E_ARTIFACT_DIR=${ARTIFACTS:-$ROOT/btc/e2e/artifacts}

cd "$ROOT"
mkdir -p "$E2E_ARTIFACT_DIR"
set +e
go test -tags e2e -count=1 -timeout 30m -v ./btc/e2e/ "$@" >"$E2E_ARTIFACT_DIR/go-test.log" 2>&1
status=$?
set -e
grep -E '^\s*(--- |PASS|FAIL|ok)' "$E2E_ARTIFACT_DIR/go-test.log" || true
echo
echo "report: $E2E_ARTIFACT_DIR/btc-e2e-report.md"
exit "$status"
