#!/usr/bin/env bash
# One-shot Homebrew install for jarvis. Puts Homebrew on PATH in login shells
# (needed on Apple Silicon, where /opt/homebrew/bin is not on the default PATH)
# then installs the formula.
#
#   /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/tranvictor/jarvis/master/scripts/install.sh)"
set -euo pipefail

say() { printf '%s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

# Write `brew shellenv` into the user's shell startup files if it is not there.
persist_brew_path() {
	local brew_exe="$1"
	local brew_bin
	brew_bin="$(dirname "$brew_exe")"
	local line="eval \"\$(${brew_exe} shellenv)\""
	local files=(
		"$HOME/.zprofile" "$HOME/.zshrc"
		"$HOME/.bash_profile" "$HOME/.bashrc"
	)
	[ "$(uname -s)" != Darwin ] && files+=("$HOME/.profile")
	local f contents
	for f in "${files[@]}"; do
		contents=""
		[ -f "$f" ] && contents="$(cat "$f")"
		case "$contents" in
		*"brew shellenv"*|*"${brew_bin}"*) continue ;;
		esac
		mkdir -p "$(dirname "$f")"
		printf '\n# Added by jarvis install.sh so the jarvis command is on PATH\n%s\n' "$line" >>"$f"
		say "Added Homebrew to PATH in $f"
	done
}

ensure_brew() {
	if command -v brew >/dev/null 2>&1; then
		return 0
	fi
	local candidate
	for candidate in /opt/homebrew/bin/brew /usr/local/bin/brew \
		"$HOME/.linuxbrew/bin/brew" /home/linuxbrew/.linuxbrew/bin/brew; do
		if [ -x "$candidate" ]; then
			# shellcheck disable=SC1090
			eval "$("$candidate" shellenv)"
			return 0
		fi
	done
	say "Homebrew is not installed. Installing it (you may be asked for your password)..."
	/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
	for candidate in /opt/homebrew/bin/brew /usr/local/bin/brew \
		"$HOME/.linuxbrew/bin/brew" /home/linuxbrew/.linuxbrew/bin/brew; do
		if [ -x "$candidate" ]; then
			# shellcheck disable=SC1090
			eval "$("$candidate" shellenv)"
			return 0
		fi
	done
	die "Homebrew installed but brew was not found on PATH. Open a new terminal and re-run this script."
}

ensure_brew
BREW_PREFIX="$(brew --prefix)"
persist_brew_path "$BREW_PREFIX/bin/brew"
eval "$("$BREW_PREFIX/bin/brew" shellenv)"

say "Installing jarvis..."
brew install tranvictor/jarvis/jarvis

if command -v jarvis >/dev/null 2>&1; then
	say ""
	say "Done. In this terminal you can run: jarvis"
else
	say ""
	say "Installed. Open a new terminal window and run: jarvis"
	say "Or in this window run:"
	say "  eval \"\$($BREW_PREFIX/bin/brew shellenv)\""
fi
