#!/usr/bin/env bash
# End-to-end check of the Classic multisig signing card against mainnet.
#
# Builds jarvis, runs `jarvis msig info` on a real Classic multisig tx inside
# a pseudo-terminal (so colours are on) at several widths, with an address
# book that gives the multisig and one signer very long names. Asserts that:
#   - every wrapped piece of a long address name is still green,
#   - addresses missing from the address book carry "(not in address book)",
#   - every box line has the same visible width.
#
# The raw ANSI transcripts are written to $OUT (default ./e2e-out) so the
# result can be inspected or replayed with `cat`.
#
# jarvis reads ~/addresses.json from the OS user's home, so this refuses to
# run when one already exists rather than touch a real address book.
set -euo pipefail

MSIG=0x2515ec2104d30a073c0ea99d916b9e20b14fc7e0
TXID=3
SIGNER=0xbe2f0354d970265bfc36d383af77f72736b81b54
RECIPIENT=0x54C8330f90BE8493fAc27995605E4ADBd25393BC
UNKNOWN_SIGNER=0x8180a5CA4E3B94045e05A9313777955f7518D757
WIDTHS=${WIDTHS:-"120 210"}
OUT=${OUT:-$PWD/e2e-out}

home=$(getent passwd "$(id -u)" | cut -d: -f6)
book="$home/addresses.json"
if [[ -e "$book" ]]; then
	echo "refusing to run: $book exists and would be replaced" >&2
	exit 2
fi
trap 'rm -f "$book"' EXIT
cat >"$book" <<EOF
{
  "$MSIG": "Lumen Main Multisig (Mike, Victor, Loi) - Eth, Bsc, Arb, Opt, Base, Pol, Robin, Avax, Bera, S, Plasma, Monad",
  "$SIGNER": "Victor Ledger Nano X - main signer for Lumen multisigs on every chain"
}
EOF

mkdir -p "$OUT"
repo=$(cd "$(dirname "$0")/.." && pwd)
bin="$OUT/jarvis"
(cd "$repo" && go build -o "$bin" .)

fail=0
for w in $WIDTHS; do
	raw="$OUT/msig-info-${w}cols.ansi"
	script -qfec "stty cols $w rows 60; $bin msig info $MSIG $TXID -k mainnet" /dev/null >"$raw" </dev/null
	if ! python3 - "$raw" "$w" "$RECIPIENT" "$UNKNOWN_SIGNER" <<'PY'; then
import re, sys, unicodedata
raw, width, recipient, unknown_signer = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4]
text = open(raw, encoding="utf-8").read().replace("\r", "")
sgr = re.compile(r"\x1b\[[0-9;]*m")
def vis(s):
    return sum(2 if unicodedata.east_asian_width(c) in "WF" else 1 for c in sgr.sub("", s))
lines = text.split("\n")
start = next(i for i, l in enumerate(lines) if "Classic multisig transaction" in l and "╭" in l)
end = next(i for i in range(start, len(lines)) if "╰" in lines[i])
box = lines[start:end + 1]
errors = []
widths = {vis(l) for l in box}
if len(widths) != 1:
    errors.append(f"box lines differ in width: {sorted(widths)}")
if max(widths) > width:
    errors.append(f"box is {max(widths)} cols, terminal is {width}")
# A name fragment must be inside green on its own line: the border before it
# resets colour, so an opener carried from the previous line does not count.
def green_at(line, idx):
    green = False
    for m in sgr.finditer(line[:idx]):
        params = m.group()[2:-1].split(";")
        for p in params:
            if p in ("", "0", "39"):
                green = False
            elif p == "32":
                green = True
    return green
seen = 0
for frag in ("Lumen", "Plasma", "Victor Ledger", "chain)"):
    for l in box:
        idx = l.find(frag)
        if idx < 0:
            continue
        seen += 1
        if not green_at(l, idx):
            errors.append(f"name fragment {frag!r} is not green: {l!r}")
if seen < 4:
    errors.append("long names were not found on the card")
plain = sgr.sub("", "\n".join(box))
flat = re.sub(r"\s*│\s*\n\s*│\s*", " ", plain)
for addr in (recipient, unknown_signer):
    if f"{addr} (not in address book)" not in flat:
        errors.append(f"{addr} is not marked as missing from the address book")
for l in box:
    if f"Recipient  {recipient}" in sgr.sub("", l) and "\x1b[33m" not in l:
        errors.append(f"recipient row is not yellow: {l!r}")
if errors:
    print(f"FAIL at {width} cols:")
    for e in errors:
        print("  - " + e)
    sys.exit(1)
print(f"PASS at {width} cols ({len(box)} box lines, {max(widths)} wide)")
PY
		fail=1
	fi
done

echo "transcripts: $OUT/msig-info-*cols.ansi (view with: cat <file>)"
exit $fail
