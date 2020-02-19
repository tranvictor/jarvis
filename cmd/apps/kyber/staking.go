package kyber

import (
	"fmt"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

var (
	StakingContract string
	DaoContract     string

	Epoch        uint64
	Stake        *big.Int
	PendingStake *big.Int
	Delegation   string
)

func Preprocess(cmd *cobra.Command, args []string) (err error) {
	switch config.Network {
	case "mainnet":
		StakingContract = ""
		DaoContract = ""
		return fmt.Errorf("'%s' doesn't have kyber staking yet", config.Network)
	case "ropsten":
		StakingContract = "0x96356c512488f8aea0751d05953bfb3e20866415"
		DaoContract = "0xb8af9107daf97cb6bdcedf977644f3cd0b0c8167"
		return nil
	}
	return fmt.Errorf("'%s' is not support for this app", config.Network)
}

var daoCmd = &cobra.Command{
	Use:               "dao",
	Short:             "Stake and participate in the KyberDAO to get reward",
	TraverseChildren:  true,
	PersistentPreRunE: Preprocess,
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(config.Network)
		if err != nil {
			fmt.Printf("Couldn't init eth reader: %s\n", err)
			return
		}
		config.From, _, err = util.GetAddressFromString(config.From)
		if err != nil {
			fmt.Printf("Couldn't interpret from address: %s\n", err)
			return
		}
		dao := NewKyberDAO(reader, StakingContract, DaoContract)
		var isCurrentEpoch bool
		if Epoch == 0 {
			Epoch, err = dao.CurrentEpoch()
			if err != nil {
				fmt.Printf("Couldn't get current epoch: %s\n", err)
				return
			}
			isCurrentEpoch = true
		}
		if isCurrentEpoch {
			fmt.Printf("Working with Epoch: %d (latest epoch)\n", Epoch)
		} else {
			fmt.Printf("Working with Epoch: %d (epoch in the past)\n", Epoch)
		}
		fmt.Printf("Staker: %s\n", util.VerboseAddress(config.From))
		stake, err := dao.GetStake(config.From, Epoch)
		if err != nil {
			fmt.Printf("Couldn't get stake of %s at epoch %d: %s\n", config.From, Epoch, err)
			return
		}
		fmt.Printf("Your stake: %f KNC (%s)\n", ethutils.BigToFloat(stake, 18), stake)
		if isCurrentEpoch {
			futureStake, err := dao.GetStake(config.From, Epoch+1)
			if err != nil {
				fmt.Printf("Couldn't get stake of %s at epoch %d: %s\n", config.From, Epoch, err)
				return
			}
			pendingStake := big.NewInt(0).Sub(futureStake, stake)
			fmt.Printf("Your pending stake (can withdraw without any penalty): %f KNC (%s)\n", ethutils.BigToFloat(pendingStake, 18), pendingStake)
		}
		poolMaster, err := dao.GetPoolMaster(config.From, Epoch)
		if err != nil {
			fmt.Printf("Couldn't get pool master info of %s at epoch %d: %s\n", config.From, Epoch, err)
			return
		}
		fmt.Printf("Your pool master: %s\n", util.VerboseAddress(poolMaster.Hex()))
		delegatedStake, err := dao.GetDelegatedStake(config.From, Epoch)
		if err != nil {
			fmt.Printf("Couldn't get delegated stake of %s at epoch %d: %s\n", config.From, Epoch, err)
			return
		}
		fmt.Printf("Stake other people delegated to you: %f KNC\n", ethutils.BigToFloat(delegatedStake, 18))
	},
}

func init() {
	daoCmd.PersistentFlags().StringVarP(&config.From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	daoCmd.PersistentFlags().Uint64VarP(&Epoch, "epoch", "e", 0, "Epoch to read staking and dao data.")

	daoCmd.MarkFlagRequired("from")
}
