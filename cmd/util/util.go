package util

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
	utilreader "github.com/tranvictor/jarvis/util/reader"
)

// classicMsigABI returns the ABI for packing Gnosis Classic multisig calls.
// It prefers the verified explorer ABI when that ABI actually describes the
// Classic methods (confirmTransaction, …) and falls back to the built-in
// ABI when the contract is unverified or is a methodless proxy.
func classicMsigABI(resolver ABIResolver, addr string, network jarvisnetworks.Network) *abi.ABI {
	fallback := util.GetGnosisMsigABI()
	if resolver == nil {
		return fallback
	}
	a, err := resolver.GetABI(addr, network)
	if err != nil || a == nil {
		return fallback
	}
	if _, ok := a.Methods["confirmTransaction"]; !ok {
		return fallback
	}
	return a
}

// PostProcessFunc is a callback called with the decoded function call after
// displaying a multisig transaction. Return an error to abort the flow.
type PostProcessFunc func(fc *jarviscommon.FunctionCall) error

// ScanForTxs scans para for network-prefixed or bare transaction hashes.
func ScanForTxs(para string) (nwks []string, addresses []string) {
	return util.ScanForTxs(para)
}

// HandleApproveOrRevokeOrExecuteMsig handles the confirm / revoke / execute
// flow for a Gnosis multisig transaction.
func HandleApproveOrRevokeOrExecuteMsig(
	u ui.UI,
	method string,
	cmd *cobra.Command,
	args []string,
	postProcess PostProcessFunc,
) {
	tc, _ := TxContextFrom(cmd)

	reader := tc.Reader
	if reader == nil {
		u.Error("Couldn't connect to blockchain.")
		return
	}

	analyzer := tc.Analyzer

	var (
		err    error
		txid   *big.Int
		txInfo *jarviscommon.TxInfo
	)

	if config.Tx == "" {
		nwks, txs := ScanForTxs(args[1])
		if len(txs) == 0 {
			txid, err = util.ParamToBigInt(args[1])
			if err != nil {
				u.Error("Invalid second param. It must be either init tx hash or tx id.")
				return
			}
		} else {
			config.Tx = txs[0]
			if nwks[0] != "" {
				if err = config.SetNetwork(nwks[0]); err != nil {
					u.Error("Not supported network: %s", err)
					return
				}
			}
		}
	}

	if txid == nil {
		txInfo = tc.TxInfo
		if txInfo == nil {
			txinfo, err := reader.TxInfoFromHash(config.Tx)
			if err != nil {
				u.Error("Couldn't get tx info from the blockchain: %s", err)
				return
			}
			txInfo = &txinfo
		}
		if txInfo.Receipt == nil {
			u.Error("Can't get receipt of the init tx. That tx might still be pending.")
			return
		}
		txid = util.GnosisMsigTxIDFromLogs(txInfo.Receipt.Logs, tc.To)
		if txid == nil && txInfo.Tx != nil {
			txid = util.GnosisMsigTxIDFromCalldata(txInfo.Tx.Data())
		}
		if txid == nil {
			u.Error("The provided tx hash is not a Classic Gnosis submit/confirm/revoke/execute tx for this multisig.")
			return
		}
	}

	multisigContract, err := msig.NewMultisigContract(tc.To, config.Network(), msig.WithReader(EthReaderOf(reader)))
	if err != nil {
		u.Error("Couldn't interact with the contract: %s", err)
		return
	}

	fc, _, _, executed, err := AnalyzeAndShowMsigTxInfo(u, multisigContract, txid, config.Network(), tc.Resolver, analyzer)
	if err != nil {
		return
	}

	if postProcess != nil && postProcess(fc) != nil {
		return
	}

	if executed {
		u.Warn("This transaction has already been executed. Nothing to do.")
		return
	}

	a := classicMsigABI(tc.Resolver, tc.To, config.Network())

	data, err := a.Pack(method, txid)
	if err != nil {
		u.Error("Couldn't pack data: %s", err)
		return
	}

	gasLimit := config.GasLimit
	if gasLimit == 0 {
		gasLimit, err = reader.EstimateExactGas(tc.From, tc.To, 0, tc.Value, data)
		if err != nil {
			var bal *big.Int
			if b, berr := reader.GetBalance(tc.From); berr == nil {
				bal = b
			}
			u.Error("%s", ExplainEstimateGasError(err, tc.From, bal, config.Network().GetNativeTokenSymbol()))
			return
		}
	}

	tx := jarviscommon.BuildExactTx(
		tc.TxType,
		tc.Nonce,
		tc.To,
		tc.Value,
		gasLimit+config.ExtraGasLimit,
		tc.GasPrice+config.ExtraGasPrice,
		tc.TipGas+config.ExtraTipGas,
		data,
		config.Network().GetChainID(),
	)

	bc := tc.Broadcaster
	if bc == nil {
		u.Error("Broadcaster not available.")
		return
	}

	SetClassicSigningNote()
	customABIs := map[string]*abi.ABI{
		strings.ToLower(tc.To): a,
	}
	if broadcasted, err := SignAndBroadcast(u, tc.FromAcc, tx, customABIs, reader, analyzer, a, bc); err != nil && !broadcasted && !errors.Is(err, ErrUserCancelled) {
		u.Error("Failed to proceed after signing the tx: %s. Aborted.", err)
	}
}

// ErrWalletUnlock is returned by SignAndBroadcast when the wallet cannot be
// unlocked. Callers that need a specific exit code (e.g. 126) can test with
// errors.Is.
var ErrWalletUnlock = errors.New("wallet unlock failed")

// SignAndBroadcast prompts the user for confirmation, unlocks the wallet,
// signs the transaction, verifies the signer, and hands off to HandlePostSign.
func SignAndBroadcast(
	u ui.UI,
	fromAcc jtypes.AccDesc,
	tx *types.Transaction,
	customABIs map[string]*abi.ABI,
	reader utilreader.Reader,
	analyzer util.TxAnalyzer,
	a *abi.ABI,
	bc TxBroadcaster,
) (bool, error) {
	note := SigningNote{WalletName: fromAcc.Desc, WalletKind: fromAcc.Kind}
	if reader != nil {
		if bal, err := reader.GetBalance(fromAcc.Address); err == nil {
			note.SignerBalance = bal
		}
	}
	mergeSigningNote(note)
	if err := PromptTxConfirmation(u, analyzer, util.GetJarvisAddress(fromAcc.Address, config.Network()), tx, customABIs, config.Network()); err != nil {
		return false, err
	}

	u.Info("Unlock your wallet and sign now…")
	account, err := accounts.UnlockAccount(fromAcc)
	if err != nil {
		return false, fmt.Errorf("%w: %s", ErrWalletUnlock, err)
	}

	signedAddr, signedTx, err := account.SignTx(tx, big.NewInt(int64(config.Network().GetChainID())))
	if err != nil {
		if errors.Is(err, trezoreum.ErrCancelled) {
			WarnCancelled(u)
			return false, ErrUserCancelled
		}
		return false, fmt.Errorf("couldn't sign tx: %w", err)
	}
	if signedAddr.Cmp(jarviscommon.HexToAddress(fromAcc.Address)) != 0 {
		return false, fmt.Errorf(
			"signed from wrong address. You could use wrong hw or passphrase. Expected wallet: %s, signed wallet: %s",
			fromAcc.Address,
			signedAddr.Hex(),
		)
	}

	return HandlePostSign(u, signedTx, reader, analyzer, a, bc)
}

type signedTxResultJSON struct {
	Tx            *types.Transaction `json:"transaction"`
	TxHash        string             `json:"txHash"`
	SenderAddress string             `json:"senderAddress"`
	SignedHex     string             `json:"signedHex"`
}

func (s *signedTxResultJSON) Write(u ui.UI, filepath string) {
	data, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		u.Error("Writing to json file failed: %s", err)
	}
}

// HandlePostSign encodes the signed transaction, optionally writes JSON output,
// and broadcasts (with optional retry) and/or waits for mining.
func HandlePostSign(
	u ui.UI,
	signedTx *types.Transaction,
	reader utilreader.Reader,
	analyzer util.TxAnalyzer,
	a *abi.ABI,
	broadcaster TxBroadcaster,
) (broadcasted bool, err error) {
	signedData, err := rlp.EncodeToBytes(signedTx)
	if err != nil {
		u.Error("couldn't encode the signed tx: %s", err)
		return false, fmt.Errorf("couldn't encode the signed tx: %w", err)
	}
	signedHex := hexutil.Encode(signedData)

	signerHex, err := jarviscommon.GetSignerAddressFromTx(
		signedTx,
		big.NewInt(int64(config.Network().GetChainID())),
	)
	if err != nil {
		return false, fmt.Errorf("couldn't derive sender address from signed tx: %w", err)
	}

	resultJSON := signedTxResultJSON{
		Tx:            signedTx,
		TxHash:        signedTx.Hash().Hex(),
		SenderAddress: signerHex.Hex(),
		SignedHex:     signedHex,
	}
	if config.JSONOutputFile != "" {
		defer resultJSON.Write(u, config.JSONOutputFile)
	}

	if config.DontBroadcast {
		u.Critical("Signed tx: %s", signedHex)
		return false, nil
	}

	if !config.RetryBroadcast {
		_, broadcasted, err := broadcaster.BroadcastTx(signedTx)
		if config.DontWaitToBeMined {
			util.DisplayBroadcastedTx(u, signedTx, broadcasted, err, config.Network())
			return broadcasted, err
		}
		util.DisplayWaitAnalyze(
			u, reader, analyzer, signedTx, broadcasted, err, config.Network(),
			a, nil, util.PostSignLayout(config.DegenMode),
		)
		return broadcasted, err
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	quit := make(chan struct{})
	broadcastedCh := make(chan *struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				_, broadcasted, err = broadcaster.BroadcastTx(signedTx)
				if broadcasted {
					broadcastedCh <- nil
					close(quit)
				} else {
					u.Error("Every RPC node rejected the tx; will retry. Details:\n%s", err)
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	<-broadcastedCh
	if config.DontWaitToBeMined {
		util.DisplayBroadcastedTx(u, signedTx, broadcasted, err, config.Network())
		return broadcasted, err
	}

	util.DisplayWaitAnalyze(
		u, reader, analyzer, signedTx, broadcasted, err, config.Network(),
		a, nil, util.PostSignLayout(config.DegenMode),
	)
	return broadcasted, err
}

// StringParamToBytes converts a hex-prefixed or raw string to bytes.
func StringParamToBytes(data string) []byte {
	if data == "" {
		return []byte{}
	}
	if strings.HasPrefix(data, "0x") {
		dataBytes, err := hex.DecodeString(data[2:])
		if err != nil {
			return []byte{}
		}
		return dataBytes
	}
	return []byte(data)
}
