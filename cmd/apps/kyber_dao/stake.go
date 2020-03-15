package kyberdao

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

var MAX_ALLOWANCE = big.NewInt(0).Lsh(big.NewInt(1), 254)

var stakeCmd = &cobra.Command{
	Use:               "stake",
	Short:             "Stake your KNC to Kyber DAO to vote and get rewards",
	TraverseChildren:  true,
	PersistentPreRunE: Preprocess,
	Run: func(cmd *cobra.Command, args []string) {
		PrintENV()

		reader, err := util.EthReader(config.Network)
		if err != nil {
			cmd.Printf("Couldn't init eth reader: %s\n", err)
			return
		}

		dao := NewKyberDAO(reader, StakingContract, DaoContract, FeeHandler)
		stakeInfo, err := dao.AllStakeRelatedInfo(config.From, Epoch)
		if err != nil {
			cmd.Printf("Couldn't get stake information: %s\n", err)
			return
		}
		PrintStakeInformation(cmd, stakeInfo)

		if stakeInfo.Balance.Cmp(big.NewInt(0)) == 0 {
			cmd.Printf("You don't have any available KNC to stake. Abort.\n")
			return
		}

		rootCmd := cmd.Root()
		nonce, err := reader.GetMinedNonce(config.From)
		if err != nil {
			cmd.Printf("Couldn't get nonce for %s: %s\n", config.From, err)
			return
		}
		OldDontWaitToBeMined := config.DontWaitToBeMined
		if stakeInfo.Allowance.Cmp(MAX_ALLOWANCE) != 0 {
			cmd.Printf("\nYou need to give KNC allowance to the staking contract.\n")
			txCmd, txArgs, err := rootCmd.Find([]string{
				"contract",
				"tx",
				KNCContract,
			})
			if err != nil {
				cmd.Printf("Couldn't find tx command: %s\n", err)
			}

			config.MethodIndex = 1
			config.Nonce = nonce
			nonce += 1
			config.PrefillStr = fmt.Sprintf("%s|%s", StakingContract, hexutil.EncodeBig(MAX_ALLOWANCE))
			config.DontWaitToBeMined = true
			txCmd.PersistentPreRunE(txCmd, txArgs)
			txCmd.Run(txCmd, txArgs)
			fmt.Printf("\n")
		}

		txCmd, txArgs, err := rootCmd.Find([]string{
			"contract",
			"tx",
			StakingContract,
		})
		if err != nil {
			cmd.Printf("Couldn't find tx command: %s\n", err)
		}
		config.MethodIndex = 2
		config.Nonce = nonce
		config.DontWaitToBeMined = OldDontWaitToBeMined
		config.GasLimit = 500000
		config.PrefillMode = false
		config.PrefillStr = ""
		txCmd.PersistentPreRunE(txCmd, txArgs)
		txCmd.Run(txCmd, txArgs)
	},
}

func init() {
	stakeCmd.PersistentFlags().Float64VarP(&config.GasPrice, "gasprice", "p", 0, "Gas price in gwei. If default value is used, we will use https://ethgasstation.info/ to get fast gas price. The gas price to be used in the tx is gas price + extra gas price")
	stakeCmd.PersistentFlags().Float64VarP(&config.ExtraGasPrice, "extraprice", "P", 0, "Extra gas price in gwei. The gas price to be used in the tx is gas price + extra gas price")
	stakeCmd.PersistentFlags().Uint64VarP(&config.Nonce, "nonce", "n", 0, "Nonce of the from account. If default value is used, we will use the next available nonce of from account")
	stakeCmd.PersistentFlags().StringVarP(&config.From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	stakeCmd.PersistentFlags().BoolVarP(&config.DontBroadcast, "dry", "d", false, "Will not broadcast the tx, only show signed tx.")
	stakeCmd.PersistentFlags().BoolVarP(&config.DontWaitToBeMined, "no-wait", "F", false, "Will not wait the tx to be mined.")
	stakeCmd.MarkFlagRequired("from")
}
