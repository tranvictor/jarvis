package kyberfpr

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	. "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

var (
	Token   string
	Reserve string
)

var KyberFPRCmd = &cobra.Command{
	Use:              "kyber-fpr",
	Short:            "utilities on FPR reserve",
	TraverseChildren: true,
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(config.Network)
		if err != nil {
			fmt.Printf("Couldn't init eth reader: %s\n", err)
			return
		}
		Reserve, resName, err := getAddressFromParams(args, 0)
		if err != nil {
			fmt.Printf("Couldn't interpret FPR reserve address: %s\n", err)
			return
		}
		fmt.Printf("Working on reserve: %s (%s)\n", Reserve, resName)
		reserve, err := NewFPRReserveContract(Reserve, reader)
		if err != nil {
			fmt.Printf("Couldn't initiate reserve instance: %s\n", err)
			return
		}
		if Token != "" {
			Token, _, err = util.GetAddressFromString(Token + " token")
			if err != nil {
				fmt.Printf("Couldn't interpret token address: %s\n", err)
				return
			}
		} else {
			tokens, err := reserve.QueryListedTokens()
			if err != nil {
				fmt.Printf("Couldn't query listed tokens: %s\n", err)
				return
			}
			if len(tokens) == 0 {
				fmt.Printf("This FPR reserve doesn't have any listed tokens.\n")
				return
			}
			fmt.Printf("\nListed tokens:\n")
			for i, token := range tokens {
				fmt.Printf("%d. %s\n", i, VerboseAddress(util.GetJarvisAddress(token.Hex(), config.Network)))
			}
			fmt.Printf("\n")

			index := util.PromptIndex("Which token do you want to check? Please enter index", 0, len(tokens)-1)

			Token = tokens[index].Hex()
		}
		fmt.Printf("\n")
		fmt.Printf("Checking on token: %s\n", VerboseAddress(util.GetJarvisAddress(Token, config.Network)))
		price, err := util.GetCoinGeckoRateInUSD(Token)
		if err != nil {
			fmt.Printf("Getting price failed: %s\n", err)
			return
		}
		err = reserve.DisplayStepFunctionData(Token, price)
		if err != nil {
			fmt.Printf("Displaying step functions failed: %s\n", err)
			return
		}
		isListed, isEnabled, err := reserve.GetBasicInfo(Token)
		if err != nil {
			fmt.Printf("Displaying token basic info failed: %s\n", err)
			return
		}
		fmt.Printf("\n")
		fmt.Printf(" Listed: %t\n", isListed)
		fmt.Printf("Enabled: %t\n", isEnabled)

		minimalRecordResolution, maxBlockImb, maxTotalImb, err := reserve.GetTokenControlInfo(Token)
		if err != nil {
			fmt.Printf("Displaying token control info failed: %s\n", err)
			return
		}

		fmt.Printf("\n")
		decimal, err := util.GetERC20Decimal(Token, config.Network)
		if err != nil {
			fmt.Printf("Getting decimal failed: %s\n", err)
			return
		}
		fmt.Printf("Price: %f USD\n", price)
		fmt.Printf(
			"Min Resolution: %s (%f USD)\n",
			ReadableNumber(minimalRecordResolution.Text(10)),
			ethutils.BigToFloat(minimalRecordResolution, decimal)*price,
		)
		fmt.Printf(
			"Max Block Imp: %s (%f USD)\n",
			ReadableNumber(maxBlockImb.Text(10)),
			ethutils.BigToFloat(maxBlockImb, decimal)*price,
		)
		fmt.Printf(
			"Max Total Imp: %s (%f USD)\n",
			ReadableNumber(maxTotalImb.Text(10)),
			ethutils.BigToFloat(maxTotalImb, decimal)*price,
		)
	},
}

var approveListingTokenCmd = &cobra.Command{
	Use:               "approve-listing-token",
	Short:             "help approving listing token request by using gnosis multisig",
	Long:              ` `,
	TraverseChildren:  true,
	PersistentPreRunE: cmdutil.CommonTxPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.HandleApproveOrRevokeOrExecuteMsig("confirmTransaction", cmd, args, func(method string, params []ParamResult, gnosisResult *GnosisResult, err error) error {
			fmt.Printf("\n\n%s\n", InfoColor("Listing token validation..."))
			if err != nil {
				fmt.Printf("%s\n", AlertColor("Provided tx doesn't call any smart contract function"))
				return fmt.Errorf("provided tx doesn't call any smart contract function")
			}

			if method != "addToken" {
				fmt.Printf("%s\n", AlertColor("Calling method is wrong. It must be 'addToken'"))
				return fmt.Errorf("wrong conversion rate contract method")
			}

			tokenValue := params[0].Value[0]

			if tokenValue.Address == nil || tokenValue.Address.Decimal == 0 {
				fmt.Printf("Token from the msig params is not an address\n")
				return fmt.Errorf("Token from the msig params is not an address")
			}

			price, err := util.GetCoinGeckoRateInUSD(tokenValue.Value)
			if err != nil {
				fmt.Printf("Getting price failed: %s\n", err)
				return fmt.Errorf("Getting price failed: %s", err)
			}

			fmt.Printf(
				"Token: %s - %s (%f USD)\n",
				tokenValue.Address.Address,
				tokenValue.Address.Desc,
				price,
			)
			resolutionBig, err := util.StringToBigInt(params[1].Value[0].Value)
			if err != nil {
				fmt.Printf("Parsing min resolution value to big in failed\n")
				return fmt.Errorf("Parsing min resolution failed: %s", err)
			}
			resolution := price * ethutils.BigToFloat(resolutionBig, tokenValue.Address.Decimal)
			if resolution > 0.01 || resolution < 0.001 {
				fmt.Printf("Resolution: %f USD (%s)\n", resolution, AlertColor("Warning"))
			} else {
				fmt.Printf("Resolution: %f USD\n", resolution)
			}

			maxPerBlockImbalanceBig, err := util.StringToBigInt(params[2].Value[0].Value)
			if err != nil {
				fmt.Printf("Parsing max block imbalance value to big in failed: %s\n", err)
				return fmt.Errorf("Parsing max block imbalance failed: %s", err)
			}
			maxPerBlockImbalance := price * ethutils.BigToFloat(maxPerBlockImbalanceBig, tokenValue.Address.Decimal)
			fmt.Printf("Max Block Imbalance: %f USD\n", maxPerBlockImbalance)

			maxTotalImbalanceBig, err := util.StringToBigInt(params[3].Value[0].Value)
			if err != nil {
				fmt.Printf("Parsing max block imbalance value to big in failed\n")
				return fmt.Errorf("Parsing max block imbalance failed: %s", err)
			}
			maxTotalImbalance := price * ethutils.BigToFloat(maxTotalImbalanceBig, tokenValue.Address.Decimal)
			fmt.Printf("Max Total Imbalance: %f USD\n", maxTotalImbalance)
			fmt.Printf("%s\n", InfoColor("==========================="))
			return nil
		})
	},
}

func init() {
	KyberFPRCmd.PersistentFlags().Int64VarP(&config.AtBlock, "block", "b", -1, "Specify the block to read at. Default value indicates reading at latest state of the chain.")
	KyberFPRCmd.PersistentFlags().StringVarP(&Token, "token", "T", "", "Token address or name of the FPR reserve to show information. If it is not specified, jarvis will show the list of listed token and you can select from them.")

	KyberFPRCmd.AddCommand(approveListingTokenCmd)
	approveListingTokenCmd.PersistentFlags().Uint64VarP(&config.ExtraGasLimit, "extragas", "G", 350000, "Extra gas limit for the tx. The gas limit to be used in the tx is gas limit + extra gas limit")
}
