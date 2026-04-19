package safe

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// GNOSIS_SAFE_ABI is the minimal subset of the GnosisSafe (and Safe v1.4.x)
// ABI needed by jarvis. It covers reading owner/threshold/nonce/version
// metadata, on-chain hash approval, and the execTransaction entry point.
//
// The full ABI is intentionally avoided to keep the binary small and to
// pin the exact methods we depend on across Safe versions (v1.1.1+).
const GNOSIS_SAFE_ABI string = `[
  {
    "inputs": [],
    "name": "getOwners",
    "outputs": [{"internalType": "address[]", "name": "", "type": "address[]"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "getThreshold",
    "outputs": [{"internalType": "uint256", "name": "", "type": "uint256"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "nonce",
    "outputs": [{"internalType": "uint256", "name": "", "type": "uint256"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "VERSION",
    "outputs": [{"internalType": "string", "name": "", "type": "string"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "domainSeparator",
    "outputs": [{"internalType": "bytes32", "name": "", "type": "bytes32"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {"internalType": "address", "name": "", "type": "address"},
      {"internalType": "bytes32", "name": "", "type": "bytes32"}
    ],
    "name": "approvedHashes",
    "outputs": [{"internalType": "uint256", "name": "", "type": "uint256"}],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {"internalType": "bytes32", "name": "hashToApprove", "type": "bytes32"}
    ],
    "name": "approveHash",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [
      {"internalType": "address", "name": "to", "type": "address"},
      {"internalType": "uint256", "name": "value", "type": "uint256"},
      {"internalType": "bytes", "name": "data", "type": "bytes"},
      {"internalType": "uint8", "name": "operation", "type": "uint8"},
      {"internalType": "uint256", "name": "safeTxGas", "type": "uint256"},
      {"internalType": "uint256", "name": "baseGas", "type": "uint256"},
      {"internalType": "uint256", "name": "gasPrice", "type": "uint256"},
      {"internalType": "address", "name": "gasToken", "type": "address"},
      {"internalType": "address", "name": "refundReceiver", "type": "address"},
      {"internalType": "bytes", "name": "signatures", "type": "bytes"}
    ],
    "name": "execTransaction",
    "outputs": [{"internalType": "bool", "name": "success", "type": "bool"}],
    "stateMutability": "payable",
    "type": "function"
  },
  {
    "inputs": [
      {"internalType": "address", "name": "to", "type": "address"},
      {"internalType": "uint256", "name": "value", "type": "uint256"},
      {"internalType": "bytes", "name": "data", "type": "bytes"},
      {"internalType": "uint8", "name": "operation", "type": "uint8"},
      {"internalType": "uint256", "name": "safeTxGas", "type": "uint256"},
      {"internalType": "uint256", "name": "baseGas", "type": "uint256"},
      {"internalType": "uint256", "name": "gasPrice", "type": "uint256"},
      {"internalType": "address", "name": "gasToken", "type": "address"},
      {"internalType": "address", "name": "refundReceiver", "type": "address"},
      {"internalType": "uint256", "name": "_nonce", "type": "uint256"}
    ],
    "name": "getTransactionHash",
    "outputs": [{"internalType": "bytes32", "name": "", "type": "bytes32"}],
    "stateMutability": "view",
    "type": "function"
  }
]`

// GetSafeABI returns a parsed copy of the Safe ABI subset.
func GetSafeABI() *abi.ABI {
	a, err := abi.JSON(strings.NewReader(GNOSIS_SAFE_ABI))
	if err != nil {
		panic(err)
	}
	return &a
}
