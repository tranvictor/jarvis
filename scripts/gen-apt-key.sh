#!/usr/bin/env bash
# One-time: generate a dedicated GPG key for signing the jarvis apt/yum repos.
#
# After this PR is merged:
# 1. Run this script (no args). It prints a private key and a public key.
# 2. Repo Settings → Secrets and variables → Actions → New repository secret
#    Name: APT_GPG_PRIVATE_KEY
#    Value: the whole "BEGIN PGP PRIVATE KEY BLOCK" … "END" block.
# 3. Repo Settings → Pages → Build and deployment → Source = GitHub Actions.
#    Ignore the Jekyll / "Static HTML" suggested workflows. The workflow that
#    deploys Pages is already in the repo: Actions → "Package repositories".
# 4. Repo Settings → Environments → github-pages → Deployment branches and tags:
#    GitHub only allows branch "master" by default. Add a **tag** rule `v*`
#    (or allow all). Without this, release-triggered deploys fail with
#    "Tag vX.Y.Z is not allowed to deploy to github-pages".
# 5. Publish a new version with `make release`. That uploads .deb/.rpm and
#    starts "Package repositories" by itself. If a run already failed, fix
#    step 4 then Actions → Package repositories → Run workflow (branch master).
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
echo "Next: add the private block as secret APT_GPG_PRIVATE_KEY,"
echo "set Pages source to GitHub Actions (do not add a Jekyll starter),"
echo "allow tag rule v* on the github-pages environment,"
echo "then publish a version with: make release"
