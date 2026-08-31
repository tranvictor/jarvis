#!/usr/bin/env bash
# Build a GPG-signed apt + yum repo tree from .deb/.rpm files.
#
# Usage:
#   scripts/build-package-repos.sh <packages-dir> <site-dir>
#
# Expects GNUPGHOME to already contain a secret key (imported by the caller).
# Optional: APT_GPG_PASSPHRASE for a protected key.
set -euo pipefail

PACKAGES="${1:?packages dir}"
SITE="${2:?site output dir}"
ORIGIN="${ORIGIN:-jarvis}"
LABEL="${LABEL:-jarvis}"
SUITE="${SUITE:-stable}"
COMPONENT="${COMPONENT:-main}"
BASE_URL="${BASE_URL:-https://tranvictor.github.io/jarvis}"

GPG_PASSPHRASE="${APT_GPG_PASSPHRASE-}"
gpg_sign() {
	# usage: gpg_sign <gpg extra args...>
	if [ -n "$GPG_PASSPHRASE" ]; then
		gpg --batch --yes --pinentry-mode loopback --passphrase "$GPG_PASSPHRASE" "$@"
	else
		gpg --batch --yes --pinentry-mode loopback "$@"
	fi
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || { echo "ERROR: missing $1" >&2; exit 1; }
}
need_cmd gzip
need_cmd gpg

keyid="$(gpg --list-secret-keys --with-colons | awk -F: '/^sec:/{print $5; exit}')"
[ -n "$keyid" ] || { echo "ERROR: no GPG secret key in GNUPGHOME" >&2; exit 1; }

shopt -s nullglob
debs=("$PACKAGES"/*.deb)
rpms=("$PACKAGES"/*.rpm)
if [ ${#debs[@]} -eq 0 ] && [ ${#rpms[@]} -eq 0 ]; then
	echo "ERROR: no .deb or .rpm files in $PACKAGES" >&2
	exit 1
fi
if [ ${#debs[@]} -gt 0 ]; then
	need_cmd dpkg-scanpackages
	need_cmd apt-ftparchive
fi
if [ ${#rpms[@]} -gt 0 ]; then
	need_cmd createrepo_c
fi

rm -rf "$SITE"
mkdir -p "$SITE/gpg" "$SITE/apt" "$SITE/rpm"

gpg --batch --yes --export "$keyid" >"$SITE/gpg/jarvis-archive-keyring.gpg"
gpg --batch --yes --armor --export "$keyid" >"$SITE/gpg/jarvis-archive-keyring.asc"

# --- apt (dists/stable layout, amd64 + arm64) ---
if [ ${#debs[@]} -gt 0 ]; then
	pool="$SITE/apt/pool/${COMPONENT}/j/jarvis"
	mkdir -p "$pool"
	cp -a "${debs[@]}" "$pool/"

	# Paths in Packages must be relative to the apt root.
	pushd "$SITE/apt" >/dev/null
	for arch in amd64 arm64; do
		bindir="dists/${SUITE}/${COMPONENT}/binary-${arch}"
		mkdir -p "$bindir"
		# dpkg-scanpackages --arch filters by the package Architecture field.
		dpkg-scanpackages --multiversion --arch "$arch" "pool/${COMPONENT}" /dev/null \
			>"$bindir/Packages"
		gzip -9kf "$bindir/Packages"
	done

	apt-ftparchive \
		-o "APT::FTPArchive::Release::Origin=${ORIGIN}" \
		-o "APT::FTPArchive::Release::Label=${LABEL}" \
		-o "APT::FTPArchive::Release::Suite=${SUITE}" \
		-o "APT::FTPArchive::Release::Codename=${SUITE}" \
		-o "APT::FTPArchive::Release::Architectures=amd64 arm64" \
		-o "APT::FTPArchive::Release::Components=${COMPONENT}" \
		-o "APT::FTPArchive::Release::Description=jarvis packages" \
		release "dists/${SUITE}" >"dists/${SUITE}/Release"

	gpg_sign --clearsign -u "$keyid" -o "dists/${SUITE}/InRelease" "dists/${SUITE}/Release"
	gpg_sign -abs -u "$keyid" -o "dists/${SUITE}/Release.gpg" "dists/${SUITE}/Release"
	popd >/dev/null
fi

# --- yum/dnf ---
rpm_gpgcheck=0
if [ ${#rpms[@]} -gt 0 ]; then
	cp -a "${rpms[@]}" "$SITE/rpm/"

	# Sign packages when rpmsign is available so dnf gpgcheck=1 works.
	# Passphrase-protected keys still sign apt Release files; rpmsign here
	# only runs for unprotected keys (the default from gen-apt-key.sh).
	if command -v rpmsign >/dev/null 2>&1 && [ -z "$GPG_PASSPHRASE" ]; then
		cat >"${HOME}/.rpmmacros" <<EOF
%_signature gpg
%_gpg_name ${keyid}
%__gpg /usr/bin/gpg
%__gpg_sign_cmd %{__gpg} gpg --batch --yes --no-armor --pinentry-mode loopback -u "%{_gpg_name}" -sbo %{__signature_filename} %{__plaintext_filename}
EOF
		for rpm in "$SITE"/rpm/*.rpm; do
			rpmsign --addsign "$rpm"
		done
		rpm_gpgcheck=1
	else
		echo "WARN: RPM packages left unsigned (missing rpmsign or key has a passphrase); repo metadata is still signed" >&2
	fi

	createrepo_c --quiet "$SITE/rpm"
	gpg_sign --detach-sign --armor -u "$keyid" -o "$SITE/rpm/repodata/repomd.xml.asc" \
		"$SITE/rpm/repodata/repomd.xml"
fi

if [ ${#debs[@]} -gt 0 ]; then
	cat >"$SITE/jarvis.list" <<EOF
deb [signed-by=/etc/apt/keyrings/jarvis-archive-keyring.gpg] ${BASE_URL}/apt ${SUITE} ${COMPONENT}
EOF
fi

if [ ${#rpms[@]} -gt 0 ]; then
	cat >"$SITE/jarvis.repo" <<EOF
[jarvis]
name=jarvis
baseurl=${BASE_URL}/rpm
enabled=1
gpgcheck=${rpm_gpgcheck}
repo_gpgcheck=1
gpgkey=${BASE_URL}/gpg/jarvis-archive-keyring.asc
EOF
fi

cat >"$SITE/index.html" <<EOF
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>jarvis packages</title>
<style>
body { font-family: system-ui, sans-serif; max-width: 44rem; margin: 2rem auto; padding: 0 1rem; line-height: 1.5; }
pre { background: #f4f4f4; padding: 1rem; overflow-x: auto; }
code { font-family: ui-monospace, monospace; }
</style>
</head>
<body>
<h1>jarvis package repository</h1>
<p>Signed apt and yum/dnf packages for <a href="https://github.com/tranvictor/jarvis">tranvictor/jarvis</a>.</p>
<h2>apt (Debian / Ubuntu)</h2>
<pre><code>sudo mkdir -p /etc/apt/keyrings
sudo curl -fsSL ${BASE_URL}/gpg/jarvis-archive-keyring.gpg \\
  -o /etc/apt/keyrings/jarvis-archive-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/jarvis-archive-keyring.gpg] ${BASE_URL}/apt ${SUITE} ${COMPONENT}" \\
  | sudo tee /etc/apt/sources.list.d/jarvis.list
sudo apt update
sudo apt install jarvis</code></pre>
<h2>dnf / yum (Fedora / RHEL / CentOS)</h2>
<pre><code>sudo rpm --import ${BASE_URL}/gpg/jarvis-archive-keyring.asc
sudo curl -fsSL ${BASE_URL}/jarvis.repo -o /etc/yum.repos.d/jarvis.repo
sudo dnf install jarvis</code></pre>
<p><a href="https://github.com/tranvictor/jarvis#installation">Full install docs</a></p>
</body>
</html>
EOF

# Prevent Jekyll from hiding files on GitHub Pages.
touch "$SITE/.nojekyll"

echo "Built package repos in $SITE (key $keyid)"
