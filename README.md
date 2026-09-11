# jarvis

Ethereum automation made easy for humans.

Jarvis is a CLI for people who operate Ethereum contracts. It decodes
transactions, builds and signs from keystores, Ledger and Trezor, and
drives Gnosis Safe and Classic multisigs from a terminal.

```text
jarvis info <tx hash>     what a transaction did, decoded
jarvis send               move ETH or tokens from a wallet or a Safe
jarvis contract read|tx   call or write any verified contract
jarvis msig               propose, approve and execute multisig txs
jarvis wc                 drive a dApp over WalletConnect v2
jarvis wallet / addr      the keys you sign with, the names you trust
```

## Support Jarvis

<p align="center">
  <a href="https://etherscan.io/address/0xe4d747cbdd6e8e5dd57db6735b6410a29f5027eb"><img src="https://img.shields.io/badge/☕_Buy_me_a_coffee-ETH_·_BSC_·_any_token-FFDD00?style=for-the-badge" alt="Buy me a coffee"></a>
</p>

<p align="center">
  <img src="docs/images/donate-qr.png" width="180" alt="QR code — scan to send any token">
</p>

<p align="center">
  <b>If Jarvis saved you a headache, buy me a coffee.</b><br>
  Send <em>any</em> token on Ethereum or BNB Smart Chain — ETH, BNB, USDC, whatever you have.
</p>

```text
0xe4d747cbdd6e8e5dd57db6735b6410a29f5027eb
```

<p align="center">
  <a href="https://etherscan.io/address/0xe4d747cbdd6e8e5dd57db6735b6410a29f5027eb"><img src="https://img.shields.io/badge/Ethereum-Etherscan-627EEA?style=for-the-badge&logo=ethereum&logoColor=white" alt="View on Etherscan"></a>
  &nbsp;
  <a href="https://bscscan.com/address/0xe4d747cbdd6e8e5dd57db6735b6410a29f5027eb"><img src="https://img.shields.io/badge/BNB_Smart_Chain-BscScan-F0B90B?style=for-the-badge&logo=binance&logoColor=black" alt="View on BscScan"></a>
</p>

Same address on both chains. Scan the QR from a wallet, or use the
copy button on the address above.

## Install

**macOS / Linux (Homebrew)** — easiest on a Mac (installs Homebrew if needed):

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/tranvictor/jarvis/master/scripts/install.sh)"
```

Open a **new** terminal and run `jarvis`. Already using Homebrew?

```bash
brew install tranvictor/jarvis/jarvis
```

<details>
<summary>Debian / Ubuntu (apt)</summary>

```bash
sudo mkdir -p /etc/apt/keyrings
sudo curl -fsSL https://tranvictor.github.io/jarvis/gpg/jarvis-archive-keyring.gpg \
  -o /etc/apt/keyrings/jarvis-archive-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/jarvis-archive-keyring.gpg] https://tranvictor.github.io/jarvis/apt stable main" \
  | sudo tee /etc/apt/sources.list.d/jarvis.list
sudo apt update && sudo apt install jarvis
```

</details>

<details>
<summary>Fedora / RHEL (dnf / yum)</summary>

```bash
sudo rpm --import https://tranvictor.github.io/jarvis/gpg/jarvis-archive-keyring.asc
sudo curl -fsSL https://tranvictor.github.io/jarvis/jarvis.repo \
  -o /etc/yum.repos.d/jarvis.repo
sudo dnf install jarvis
```

</details>

<details>
<summary>Windows (Scoop)</summary>

```powershell
scoop bucket add tranvictor https://github.com/tranvictor/homebrew-tranvictor
scoop install jarvis
```

</details>

PATH, upgrades, Ledger udev rules and building from source:
[docs/install.md](docs/install.md).

## Quick start

```bash
jarvis -h
jarvis info 0x…                    # decode a tx
jarvis send                        # ETH or tokens, EOA or Safe
jarvis msig summary 0xSAFE         # pending Safe / Classic txs
jarvis wc "wc:…" --from me         # pair a dApp
```

| Guide | What it covers |
|-------|----------------|
| [Output](docs/output.md) | How `info`, signing cards and batch runs read |
| [Multisig](docs/multisig.md) | Safe + Classic via one `jarvis msig` command |
| [WalletConnect](docs/walletconnect.md) | Drive a dApp from the terminal |
| [Install extras](docs/install.md) | From source, udev, custom RPC nodes |

Nodes and networks: `jarvis node` / `jarvis network` (stored under
`~/.jarvis/`). Pick a chain with `-k/--network`.
