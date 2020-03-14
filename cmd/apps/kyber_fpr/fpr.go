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
		Token, tokenName, err := getAddressFromParams(args, 1)
		if err != nil {
			fmt.Printf("Couldn't interpret token address: %s\n", err)
			return
		}
		fmt.Printf("Checking token: %s (%s)\n", Token, tokenName)
		reserve, err := NewFPRReserveContract(Reserve, reader)
		if err != nil {
			fmt.Printf("Couldn't initiate reserve instance: %s\n", err)
			return
		}
		err = reserve.DisplayStepFunctionData(Token)
		if err != nil {
			fmt.Printf("Displaying step functions failed: %s\n", err)
		}
	},
}

func init() {
	KyberFPRCmd.PersistentFlags().Int64VarP(&config.AtBlock, "block", "b", -1, "Specify the block to read at. Default value indicates reading at latest state of the chain.")
}
