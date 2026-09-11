# Install and build

Packaged installs are on the [README](../README.md#install). This page covers
upgrades, PATH quirks, building from source, Ledger udev rules, and RPC nodes.

## Homebrew extras (macOS and Linux)

On Apple Silicon, Homebrew lives in `/opt/homebrew/bin`, which is not on
macOS's default PATH. The formula appends `brew shellenv` to zsh
(`~/.zprofile`, `~/.zshrc`) and bash (`~/.bash_profile`, `~/.bashrc`) so
`jarvis` works in new terminals. Already installed? Run
`brew reinstall tranvictor/jarvis/jarvis` once, or:

```bash
eval "$(/opt/homebrew/bin/brew shellenv)"
```

To upgrade, refresh the tap first. `brew upgrade jarvis` alone uses the
locally cached formula, so after a new GitHub release it can report:

`Warning: tranvictor/jarvis/jarvis 0.1.0 already installed`

```bash
brew update
brew upgrade tranvictor/jarvis/jarvis
```

Linux users who already have [Homebrew](https://docs.brew.sh/Homebrew-on-Linux)
can use the same `brew install` / `brew upgrade` commands; the tap ships
Linux amd64 and arm64 bottles.

## Linux (apt) extras

Upgrade with `sudo apt update && sudo apt install --only-upgrade jarvis`.

The apt (and dnf) repo is published to GitHub Pages on each GitHub release
(`make release`). Until that has happened once after packages were added,
the URLs in the README will 404.

## Linux (dnf / yum) extras

Upgrade with `sudo dnf upgrade jarvis`.

## Windows (Scoop) extras

Upgrade with `scoop update jarvis`.

Jarvis works in cmd and PowerShell but will not have color.
[Windows Terminal](https://apps.microsoft.com/detail/9n0dx20hk701) and
[Git Bash](https://gitforwindows.org/) support color.

## Build from source

Needs [Go](https://go.dev/dl/) 1.25+ (see `go.mod`). Hardware wallets need
cgo (`CGO_ENABLED=1`) and a C toolchain (Xcode CLT on macOS, `build-essential`
on Linux, mingw-w64 on Windows).

```bash
git clone https://github.com/tranvictor/jarvis.git
cd jarvis
go build -trimpath -o jarvis .
```

Or, without cloning:

```bash
CGO_ENABLED=1 go install github.com/tranvictor/jarvis@latest
```

The binary lands in `$(go env GOPATH)/bin` (often `~/go/bin`). Put that
directory on your `PATH`, then run `jarvis --help`.

If module download errors, clear the cache at `$(go env GOPATH)/pkg/mod`
and retry.

### Windows toolchain

Install [mingw-w64](https://www.mingw-w64.org/), add its `bin` folder to
`PATH`, then `go build -v` in the repo. You should get `jarvis.exe`.

## Ledger on Ubuntu

Add udev rules and reload. See
[Ledger's connection-issue guide](https://support.ledger.com/hc/en-us/articles/115005165269-Fix-connection-issues).

```bash
wget -q -O - https://raw.githubusercontent.com/LedgerHQ/udev-rules/master/add_udev_rules.sh | sudo bash
```

## RPC nodes and networks

Nodes live in `~/.jarvis/nodes/<network>.json`. Manage them with
`jarvis node` instead of editing JSON by hand:

```bash
jarvis node list
jarvis node add mainnet mynode https://my-rpc.example
jarvis node test mainnet
```

`jarvis network list` shows chain IDs. Built-in defaults can be overridden
per network, or with an env var such as `ETHEREUM_MAINNET_NODE`.

An old `~/nodes.json` is migrated automatically to `~/.jarvis/nodes/` on
first run (the original is renamed `~/nodes.json.bak`).
