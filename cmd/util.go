package cmd

import (
	"fmt"
	"math/big"

	"github.com/Songmu/prompter"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util"
)

func showTxInfoToConfirm(from string, tx *types.Transaction) error {
	fmt.Printf(
		"From: %s ==> %s\n",
		util.VerboseAddress(from, config.Network),
		util.VerboseAddress(tx.To().Hex(), config.Network),
	)

	sendingETH := ethutils.BigToFloat(tx.Value(), 18)
	if sendingETH > 0 {
		fmt.Printf("Value: %s\n", util.InfoColor(fmt.Sprintf("%f ETH", sendingETH)))
	}

	fmt.Printf(
		"Nonce: %d  |  Gas: %.2f gwei (%d gas, %f ETH)\n",
		tx.Nonce(),
		ethutils.BigToFloat(tx.GasPrice(), 9),
		tx.Gas(),
		ethutils.BigToFloat(
			big.NewInt(0).Mul(big.NewInt(int64(tx.Gas())), tx.GasPrice()),
			18,
		),
	)
	var a *abi.ABI
	var err error
	if config.ForceERC20ABI {
		a, err = ethutils.GetERC20ABI()
	} else {
		a, err = util.GetABI(tx.To().Hex(), config.Network)
	}
	if err != nil {
		return fmt.Errorf("Getting abi of the contract failed: %s", err)
	}
	analyzer := txanalyzer.NewAnalyzer()
	method, params, gnosisResult, err := analyzer.AnalyzeMethodCall(a, tx.Data())
	if err != nil {
		return fmt.Errorf("Can't decode method call: %s", err)
	}
	fmt.Printf("\nContract: %s\n", util.VerboseAddress(tx.To().Hex(), config.Network))
	fmt.Printf("Method: %s\n", method)
	for _, param := range params {
		fmt.Printf(
			" . %s (%s): %s\n",
			param.Name,
			param.Type,
			util.DisplayValues(param.Value, config.Network),
		)
	}
	util.PrintGnosis(gnosisResult)
	return nil
}

func promptTxConfirmation(from string, tx *types.Transaction) error {
	fmt.Printf("\n========== Confirm tx data before signing ==========\n\n")
	showTxInfoToConfirm(from, tx)
	if !prompter.YN("\nConfirm?", true) {
		return fmt.Errorf("user aborted")
	}
	return nil
}

func indent(nospace int, strs []string) string {
	if len(strs) == 0 {
		return ""
	}

	if len(strs) == 1 {
		return strs[0]
	}

	indentation := ""
	for i := 0; i < nospace; i++ {
		indentation += " "
	}
	result := ""
	for i, str := range strs {
		result += fmt.Sprintf("\n%s%d. %s", indentation, i, str)
	}
	result += "\n"
	return result
}
