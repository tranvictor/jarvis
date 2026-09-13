package cmd

import (
	"context"
	"encoding/hex"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/account"
	"github.com/tranvictor/jarvis/vet"
)

var vetCmd = &cobra.Command{
	Use:   "vet [address]",
	Short: "Review a contract call or typed-data Permit without signing",
	Long: `vet runs the same analysis --careful adds to a signing card:
local danger checks, verified source, and Grok when XAI_API_KEY is set.

  jarvis vet <address>                 pick a method interactively
  jarvis vet <address> --data 0x...    review already-encoded calldata
  jarvis vet --typed-data file.json    review eth_signTypedData_v4 JSON

Address-book names stay on the terminal and are never sent to Grok.`,
	Args: cobra.MaximumNArgs(1),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if config.VetTypedData != "" {
			return cmdutil.CommonNetworkPreprocess(appUI, cmd, args)
		}
		return cmdutil.CommonFunctionCallPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		if config.VetTypedData != "" {
			runVetTypedData()
			return
		}
		runVetCall(cmd)
	},
}

func runVetTypedData() {
	raw, err := os.ReadFile(config.VetTypedData)
	if err != nil {
		appUI.Error("couldn't read typed-data file: %s", err)
		return
	}
	td, err := account.ParseTypedDataV4(raw)
	if err != nil {
		appUI.Error("%s", err)
		return
	}
	req := vet.TypedRequest{
		Mode:           vet.ModeFull,
		ChainID:        config.Network().GetChainID(),
		NetworkName:    config.Network().GetName(),
		NetworkChainID: config.Network().GetChainID(),
		PrimaryType:    td.PrimaryType,
		Verifying:      td.Domain.VerifyingContract,
		Message:        td.Message,
		Book:           cmdutil.AddressBook(),
		Chain:          cmdutil.ExplorerLookupFor(config.Network()),
		AI:             cmdutil.GrokCompleter(),
	}
	if td.Domain.ChainId != nil {
		req.DomainChainID = (*big.Int)(td.Domain.ChainId)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	cmdutil.PrintVetReport(appUI, vet.AnalyzeTypedData(ctx, req))
}

func runVetCall(cmd *cobra.Command) {
	tc, _ := cmdutil.TxContextFrom(cmd)
	to := tc.To
	var data []byte
	var err error
	if config.VetCalldata != "" {
		hexStr := strings.TrimPrefix(config.VetCalldata, "0x")
		data, err = hex.DecodeString(hexStr)
		if err != nil {
			appUI.Error("couldn't decode --data: %s", err)
			return
		}
	} else {
		if to == "" {
			appUI.Error("vet needs a contract address or --typed-data")
			return
		}
		a, aerr := tc.Resolver.ConfigToABI(to, config.ForceERC20ABI, config.CustomABI, config.Network())
		if aerr != nil {
			appUI.Error("Couldn't get abi for %s: %s", to, aerr)
			return
		}
		data, err = cmdutil.PromptTxData(
			appUI, tc.Analyzer, to, config.MethodIndex,
			tc.PrefillParams, tc.PrefillMode, a, nil, config.Network(),
		)
		if err != nil {
			appUI.Error("Couldn't pack data: %s", err)
			return
		}
	}

	dest := util.GetJarvisAddress(to, config.Network())
	var fc *jarviscommon.FunctionCall
	if len(data) > 0 && to != "" {
		fc = tc.Analyzer.AnalyzeFunctionCallRecursively(util.GetABI, tc.Value, to, data, nil)
	}
	appUI.Info("Contract: %s", dest.Address)
	if fc != nil && fc.Method != "" {
		appUI.Info("Method: %s", fc.Method)
	}
	if to != "" {
		appUI.Info("To: %s", common.HexToAddress(to).Hex())
	}
	req := cmdutil.FullVetRequest(config.Network(), dest, tc.Value, data, fc)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	progress := appUI.Spinner("vet: analysing…")
	report := vet.Analyze(ctx, req)
	progress.Stop(ui.StyledText{})
	cmdutil.PrintVetReport(appUI, report)
}

func init() {
	vetCmd.Flags().StringVar(&config.VetCalldata, "data", "", "hex calldata to review instead of prompting for a method")
	vetCmd.Flags().StringVar(&config.VetTypedData, "typed-data", "", "path to an eth_signTypedData_v4 JSON file")
	vetCmd.Flags().StringVarP(&config.PrefillStr, "prefills", "I", "", "Prefill params string. Each param is separated by | char.")
	vetCmd.Flags().Uint64VarP(&config.MethodIndex, "method-index", "M", 0, "Index of the method in the alphabetically sorted method list (from 1).")
	vetCmd.Flags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
	vetCmd.Flags().StringVarP(&config.CustomABI, "abi", "c", "", customABIFlagHelp)
	rootCmd.AddCommand(vetCmd)
}
