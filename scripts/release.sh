#!/usr/bin/env bash
# Interactive release: draft notes, commit leftovers, tag, push, goreleaser.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

NOTES_FILE="$ROOT/.release-notes.md"
CROSS_IMAGE="ghcr.io/goreleaser/goreleaser-cross:${GORELEASER_CROSS_VERSION:-v1.23.2}"
SKIP_UNTRACKED_RE='(^|/)(\.DS_Store|tags)$'

say() { printf '%s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

ask() {
	local prompt="$1" default="${2-}" reply
	if [ -n "$default" ]; then
		printf '%s [%s]: ' "$prompt" "$default" >/dev/tty
	else
		printf '%s: ' "$prompt" >/dev/tty
	fi
	IFS= read -r reply </dev/tty || true
	if [ -z "$reply" ]; then
		printf '%s' "$default"
	else
		printf '%s' "$reply"
	fi
}

confirm() {
	local prompt="$1" default="${2:-n}" reply yn
	if [ "$default" = y ]; then yn="Y/n"; else yn="y/N"; fi
	printf '%s [%s]: ' "$prompt" "$yn" >/dev/tty
	IFS= read -r reply </dev/tty || true
	reply="${reply:-$default}"
	[[ "$reply" =~ ^[Yy]$ ]]
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

need_docker() {
	need_cmd docker
	docker info >/dev/null 2>&1 || die "docker is not running. Start Docker Desktop (or the daemon), then re-run make release."
}

run_goreleaser() {
	local version="$1"
	need_docker
	say "Running goreleaser-cross (cgo + GitHub + Homebrew tap + Scoop)..."
	if ! docker run --rm --privileged \
		-e CGO_ENABLED=1 \
		-e GITHUB_TOKEN \
		-v "$ROOT:/go/src/github.com/tranvictor/jarvis" \
		-w /go/src/github.com/tranvictor/jarvis \
		"$CROSS_IMAGE" \
		release --clean --release-notes .release-notes.md
	then
		die "goreleaser failed. Tag $version is already on origin — fix the error, then re-run make release."
	fi
	say ""
	say "Released $version"
	say "https://github.com/tranvictor/jarvis/releases/tag/$version"
}

write_notes_from_tag() {
	local version="$1"
	git tag -l --format='%(contents)' "$version" | python3 -c '
import sys
from pathlib import Path
text = sys.stdin.read().strip() + "\n"
Path(sys.argv[1]).write_text(text, encoding="utf-8")
' "$NOTES_FILE"
	[ -s "$NOTES_FILE" ] || die "tag $version has no annotation; cannot resume"
}

# EDITOR/VISUAL may point at a broken Homebrew MacVim (missing libruby).
# Try those first, then system vim / nano / vi.
open_editor() {
	local file="$1" editor
	local -a editor_cmd tried=() candidates=()

	[ -n "${RELEASE_EDITOR:-}" ] && candidates+=("$RELEASE_EDITOR")
	[ -n "${VISUAL:-}" ] && candidates+=("$VISUAL")
	[ -n "${EDITOR:-}" ] && candidates+=("$EDITOR")
	candidates+=("/usr/bin/vim" "nano" "vi")

	local seen=" "
	for editor in "${candidates[@]}"; do
		[ -n "$editor" ] || continue
		case "$seen" in
			*" $editor "*) continue ;;
		esac
		seen+="$editor "

		# EDITOR may be "code -w"; split so flags work.
		read -r -a editor_cmd <<<"$editor"
		command -v "${editor_cmd[0]}" >/dev/null 2>&1 || continue
		say "Opening $editor on $file — edit the notes, save, and quit."
		if "${editor_cmd[@]}" "$file" </dev/tty >/dev/tty 2>/dev/tty; then
			return 0
		fi
		say "Editor '$editor' failed. Trying another..."
		tried+=("$editor")
	done
	die "no working editor (tried: ${tried[*]}). Set RELEASE_EDITOR=/usr/bin/vim or nano."
}

normalize_version() {
	local v="$1"
	v="${v#v}"
	[[ "$v" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || return 1
	printf 'v%s' "$v"
}

bump_patch() {
	local v="${1#v}"
	local maj min pat
	IFS=. read -r maj min pat <<<"$v"
	printf 'v%s.%s.%s' "$maj" "$min" "$((pat + 1))"
}

repo_dirty() {
	[ -n "$(git status --porcelain)" ]
}

untracked_files() {
	git ls-files --others --exclude-standard
}

[ -t 0 ] || [ -r /dev/tty ] || die "make release needs a terminal (it prompts for notes, version, and confirmations)"

need_cmd git
need_docker
need_cmd python3

[ -n "${GITHUB_TOKEN:-}" ] || die "GITHUB_TOKEN is not set.

Create a GitHub token with repo access (classic) or Contents read/write on
jarvis + the homebrew-tranvictor tap (Homebrew formula and Scoop bucket), then:

  export GITHUB_TOKEN=ghp_..."

git rev-parse --is-inside-work-tree >/dev/null 2>&1 || die "not a git repo"

branch="$(git branch --show-current)"
[ -n "$branch" ] || die "detached HEAD; check out master (or a release branch) first"

if [ "$branch" != master ]; then
	say "Current branch is '$branch', not master."
	confirm "Release from $branch anyway?" n || die "aborted"
fi

say "Fetching origin..."
# --force: origin tags are source of truth; Git otherwise refuses to update
# local tags that already exist ("would clobber existing tag") and aborts.
git fetch origin --tags --prune --force

upstream="$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null || true)"
if [ -z "$upstream" ]; then
	die "branch '$branch' has no upstream. Set one (git push -u origin $branch) first."
fi

behind="$(git rev-list --count "HEAD..@{u}")"
if [ "$behind" != 0 ]; then
	die "$branch is behind $upstream by $behind commit(s). Pull (or rebase) first."
fi

last_tag="$(git describe --tags --abbrev=0 2>/dev/null || true)"
[ -n "$last_tag" ] || die "no existing tag to compare against"

if [ "$(git rev-parse HEAD)" = "$(git rev-parse "$last_tag^{commit}")" ]; then
	say "HEAD is already $last_tag."
	say "A previous run likely tagged but did not finish publishing (e.g. Docker was down)."
	if repo_dirty; then
		say ""
		say "Working tree is dirty; goreleaser needs a clean tree matching $last_tag:"
		git status --short
		confirm "Stash these changes, publish $last_tag, then restore the stash?" y || die "aborted"
		git stash push -u -m "make-release resume $last_tag"
		trap 'git stash pop || true' EXIT
	fi
	write_notes_from_tag "$last_tag"
	say ""
	say "=== Resume $last_tag ==="
	sed 's/^/  /' "$NOTES_FILE"
	say ""
	confirm "Publish $last_tag with goreleaser (tag already on origin)?" y || die "aborted"
	run_goreleaser "$last_tag"
	exit 0
fi

say ""
say "=== Changes since $last_tag ==="
if [ "$(git rev-parse HEAD)" = "$(git rev-parse "$last_tag^{commit}")" ]; then
	say "(no commits yet — only uncommitted work)"
else
	git log --oneline "$last_tag"..HEAD
fi
if repo_dirty; then
	say ""
	say "=== Uncommitted ==="
	git status --short
fi
say ""

python3 "$ROOT/scripts/compose-release-notes.py" "$last_tag" "$NOTES_FILE"
say "PR bodies are included when GITHUB_TOKEN can read them; trim anything you don't want published."
open_editor "$NOTES_FILE"

python3 - "$NOTES_FILE" <<'PY'
import re, sys
from pathlib import Path
p = Path(sys.argv[1])
text = re.sub(r"<!--.*?-->", "", p.read_text(encoding="utf-8"), flags=re.S)
text = re.sub(r"\n{3,}", "\n\n", text).strip() + "\n"
p.write_text(text, encoding="utf-8")
PY
[ -s "$NOTES_FILE" ] || die "release notes are empty; aborted"

if repo_dirty; then
	say ""
	say "Working tree still has local changes:"
	git status --short
	confirm "Commit these before tagging?" y || die "aborted (goreleaser requires a clean tree)"
	git add -u
	while IFS= read -r f; do
		[ -n "$f" ] || continue
		if [[ "$f" =~ $SKIP_UNTRACKED_RE ]]; then
			say "skipping untracked junk: $f"
			continue
		fi
		git add -- "$f"
	done < <(untracked_files)
	if git diff --cached --quiet; then
		die "nothing staged (leftover files were skipped). Clean or add them yourself and re-run."
	fi
	msg="$(ask "Commit message" "Prepare release")"
	[ -n "$msg" ] || die "empty commit message"
	git commit -m "$msg"
fi

if repo_dirty; then
	die "working tree is still dirty after commit. Untracked junk may remain; remove or ignore it, then re-run."
fi

suggested="$(bump_patch "$last_tag")"
say ""
raw_version="$(ask "Version" "$suggested")"
version="$(normalize_version "$raw_version")" || die "version must look like v0.2.2 (got: $raw_version)"

if git rev-parse "$version" >/dev/null 2>&1; then
	die "tag $version already exists locally"
fi
if git ls-remote --exit-code origin "refs/tags/$version" >/dev/null 2>&1; then
	die "tag $version already exists on origin"
fi

ahead="$(git rev-list --count "@{u}..HEAD")"
say ""
say "=== Release plan ==="
say "  branch:  $branch (will push $ahead commit(s) to $upstream)"
say "  tag:     $version (annotated, from $(git rev-parse --short HEAD))"
say "  image:   $CROSS_IMAGE"
say "  notes:   $NOTES_FILE"
say ""
sed 's/^/  /' "$NOTES_FILE"
say ""
confirm "Tag, push $branch + $version, and publish with goreleaser?" n || die "aborted"

git tag -a "$version" -F "$NOTES_FILE"
say "Pushing $branch and $version..."
git push origin "HEAD:$branch"
git push origin "$version"

run_goreleaser "$version"
