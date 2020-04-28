package kyberfpr

import (
	"fmt"

	"github.com/spf13/cobra"
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
				fmt.Printf("%d. %s\n", i, util.VerboseAddress(token.Hex(), config.Network))
			}
			fmt.Printf("\n")

			index := util.PromptIndex("Which token do you want to check? Please enter index", 0, len(tokens)-1)

			Token = tokens[index].Hex()
		}
		fmt.Printf("\n")
		fmt.Printf("Checking on token: %s\n", util.VerboseAddress(Token, config.Network))
		err = reserve.DisplayStepFunctionData(Token)
		if err != nil {
			fmt.Printf("Displaying step functions failed: %s\n", err)
		}
	},
}

func init() {
	KyberFPRCmd.PersistentFlags().Int64VarP(&config.AtBlock, "block", "b", -1, "Specify the block to read at. Default value indicates reading at latest state of the chain.")
	KyberFPRCmd.PersistentFlags().StringVarP(&Token, "token", "T", "", "Token address or name of the FPR reserve to show information. If it is not specified, jarvis will show the list of listed token and you can select from them.")
}
