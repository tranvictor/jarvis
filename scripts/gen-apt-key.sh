#!/usr/bin/env bash
# One-time: generate a dedicated GPG key for signing the jarvis apt/yum repos.
#
# 1. Run this script.
# 2. Add the printed private key as GitHub secret APT_GPG_PRIVATE_KEY
#    (repo Settings → Secrets and variables → Actions).
# 3. Enable GitHub Pages on this repo: Settings → Pages → Source = GitHub Actions.
# 4. Re-run the "Package repositories" workflow (or cut the next release).
#
# Optional: if you pass a passphrase to this script, also add it as
# APT_GPG_PASSPHRASE. An unprotected key is fine — the GitHub secret is the
# protection.
set -euo pipefail

PASSPHRASE="${1-}"
NAME="${JARVIS_PACKAGING_NAME:-jarvis packaging}"
EMAIL="${JARVIS_PACKAGING_EMAIL:-vu.tran54@gmail.com}"
GNUPGHOME="$(mktemp -d)"
export GNUPGHOME
chmod 700 "$GNUPGHOME"
cleanup() { rm -rf "$GNUPGHOME"; }
trap cleanup EXIT

batch="$(mktemp)"
{
	printf 'Key-Type: RSA\n'
	printf 'Key-Length: 4096\n'
	printf 'Key-Usage: sign\n'
	printf 'Name-Real: %s\n' "$NAME"
	printf 'Name-Email: %s\n' "$EMAIL"
	printf 'Expire-Date: 0\n'
	if [ -n "$PASSPHRASE" ]; then
		printf 'Passphrase: %s\n' "$PASSPHRASE"
	else
		printf '%%no-protection\n'
	fi
} >"$batch"

gpg --batch --generate-key "$batch"
rm -f "$batch"

keyid="$(gpg --list-secret-keys --with-colons | awk -F: '/^sec:/{print $5; exit}')"
[ -n "$keyid" ] || { echo "ERROR: failed to generate a key" >&2; exit 1; }

echo "=== GitHub secret APT_GPG_PRIVATE_KEY (private — do not commit) ==="
gpg --armor --export-secret-keys "$keyid"
echo
echo "=== Public key (also exported automatically on each Pages deploy) ==="
gpg --armor --export "$keyid"
echo
if [ -n "$PASSPHRASE" ]; then
	echo "Also add GitHub secret APT_GPG_PASSPHRASE with the passphrase you passed."
fi
echo "Key ID: $keyid"
echo "Add the private block as APT_GPG_PRIVATE_KEY, then enable GitHub Pages (Actions source)."
