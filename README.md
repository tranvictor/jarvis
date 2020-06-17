# jarvis
Ethereum automation made easy to human

## Installation
### MacOS via Homebrew
```bash
brew install tranvictor/jarvis/jarvis
```
or to upgrade jarvis to the latest version
```bash
brew upgrade jarvis
```

## Build from source

### Ubuntu Build

```bash
sudo add-apt-repository ppa:longsleep/golang-backports
sudo apt-get update
sudo apt-get install go-1.12
GO111MODULE=on /usr/lib/go-1.12/bin/go get github.com/tranvictor/jarvis@v0.0.1
GO111MODULE=on /usr/lib/go-1.12/bin/go install github.com/tranvictor/jarvis
```

`jarvis` command will be installed to `~/go/bin`

### MacOS Build
1. Download and install Go v1.12 [here](https://golang.org/dl/)
```
GO111MODULE=on go get github.com/tranvictor/jarvis
GO111MODULE=on go install github.com/tranvictor/jarvis
```

`jarvis` binary file will be placed at `$GOPATH/bin/`

If the installation process returned errors, try to clear Go module cache at `$GOPATH/pkg/mod`

2. Try `$GOPATH/bin/jarvis --help`
3. Add Jarvis to PATH, and relevant `addresses.json`


### Windows Build
Install mingw-w64 from [here](https://sourceforge.net/projects/mingw-w64/files/Toolchains%20targetting%20Win32/Personal%20Builds/mingw-builds/installer/mingw-w64-install.exe/download)
Add mingw-64 bin folder to PATH
Go to jarvis folder and using following command to build
```
go build -v
```
There should be jarvis.exe file. Add jarvis to PATH (optional)

Jarvis works on cmd, proshell but will not  have color. 
[Windows Terminal](https://www.microsoft.com/en-us/p/windows-terminal-preview/9n0dx20hk701?activetab=pivot:overviewtab) and [Gitbash](https://gitforwindows.org/) support color


## How to use it

See help with
```
~/go/bin/jarvis -h
```

## Ledger on Ubuntu
Add the rules and reload udev. More infomation see [here](https://support.ledger.com/hc/en-us/articles/115005165269-Fix-connection-issues)
```
wget -q -O - https://raw.githubusercontent.com/LedgerHQ/udev-rules/master/add_udev_rules.sh | sudo bash
```
