#!/usr/bin/env sh
# assert-cgo.sh <path-to-built-binary>
#
# Release guard: fail loudly if a jarvis binary was built without cgo.
#
# The USB / HID layer that talks to Ledger and Trezor hardware wallets
# (util/account/usb) falls back to a silent no-op stub when CGO_ENABLED=0
# (see util/account/usb/usb_disabled.go: Enumerate returns no devices and no
# error). Such a binary looks healthy but can NEVER detect a hardware wallet.
# Go records the effective CGO_ENABLED in the embedded build info, so we assert
# it here for every artifact goreleaser produces.
set -eu

bin="${1:-}"
if [ -z "$bin" ]; then
	echo "assert-cgo.sh: missing binary path argument" >&2
	exit 2
fi

if ! go version -m "$bin" | grep -q 'CGO_ENABLED=1'; then
	echo "FATAL: $bin was built without cgo (CGO_ENABLED!=1)." >&2
	echo "       USB / hardware-wallet support would be silently disabled." >&2
	echo "       Ensure this target sets CGO_ENABLED=1 and a matching CC in .goreleaser.yml." >&2
	exit 1
fi

echo "cgo OK: $bin"
