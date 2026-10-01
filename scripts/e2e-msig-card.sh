#!/usr/bin/env bash
# End-to-end check of the Classic multisig signing card.
#
# Usage: scripts/e2e-msig-card.sh <classic-msig-address> <txid> [network]
#
# Builds jarvis and runs `jarvis msig info` inside a pseudo-terminal (so
# colours are on) at several widths, with a throwaway address book that gives
# the multisig a long synthetic name. Asserts that:
#   - every wrapped piece of that name is still green,
#   - every address on the card without a name carries "(not in address book)",
#   - every box line has the same visible width.
#
# No real address or name is stored here: the multisig comes from the
# command line and its fixture name is a placeholder. The raw ANSI
# transcripts are written to $OUT (default ./e2e-out); keep them private if
# the multisig is yours.
#
# jarvis reads ~/addresses.json from the OS user's home, so this refuses to
# run when one already exists rather than touch a real address book.
set -euo pipefail

if [[ $# -lt 2 ]]; then
	echo "usage: $0 <classic-msig-address> <txid> [network]" >&2
	exit 2
fi
MSIG=$1
TXID=$2
NETWORK=${3:-mainnet}
WIDTHS=${WIDTHS:-"120 210"}
OUT=${OUT:-$PWD/e2e-out}
NAME="Placeholder Multisig Name (Alpha, Bravo, Charlie) - Delta, Echo, Foxtrot, Golf, Hotel, India, Juliett, Kilo, Lima, Mike, Oscar"

home=$(getent passwd "$(id -u)" | cut -d: -f6)
book="$home/addresses.json"
if [[ -e "$book" ]]; then
	echo "refusing to run: $book exists and would be replaced" >&2
	exit 2
fi
trap 'rm -f "$book"' EXIT
printf '{\n  "%s": "%s"\n}\n' "$MSIG" "$NAME" >"$book"

mkdir -p "$OUT"
repo=$(cd "$(dirname "$0")/.." && pwd)
bin="$OUT/jarvis"
(cd "$repo" && go build -o "$bin" .)

fail=0
for w in $WIDTHS; do
	raw="$OUT/msig-info-${w}cols.ansi"
	script -qfec "stty cols $w rows 60; $bin msig info $MSIG $TXID -k $NETWORK" /dev/null >"$raw" </dev/null
	if ! python3 - "$raw" "$w" <<'PY'; then
import re, sys, unicodedata
raw, width = sys.argv[1], int(sys.argv[2])
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
        for p in m.group()[2:-1].split(";"):
            if p in ("", "0", "39"):
                green = False
            elif p == "32":
                green = True
    return green
seen = set()
for frag in ("Placeholder", "Juliett", "Oscar)"):
    for l in box:
        idx = l.find(frag)
        if idx < 0:
            continue
        seen.add(frag)
        if not green_at(l, idx):
            errors.append(f"name fragment {frag!r} is not green: {sgr.sub('', l)!r}")
if len(seen) < 3:
    errors.append("the fixture name was not found on the card")
body = [l for l in box if not sgr.sub("", l).strip("│ ").startswith("!")]
flat = re.sub(r"\s*│\s*\n\s*│\s*", " ", sgr.sub("", "\n".join(body)))
for m in re.finditer(r"0x[0-9a-fA-F]{40}(?![0-9a-fA-F])", flat):
    if not flat[m.end():].startswith(" ("):
        errors.append(f"{m.group()} has neither a name nor the not-in-address-book mark")
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
