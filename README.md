# jarvis
Ethereum automation made easy to human

## Installation

## Build on your own

```bash
sudo add-apt-repository ppa:longsleep/golang-backports
sudo apt-get update
sudo apt-get install go-1.12
GO111MODULE=on /usr/lib/go-1.12/bin/go get github.com/tranvictor/jarvis@v0.0.1
GO111MODULE=on /usr/lib/go-1.12/bin/go install github.com/tranvictor/jarvis
```

`jarvis` command will be installed to `~/go/bin`

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
