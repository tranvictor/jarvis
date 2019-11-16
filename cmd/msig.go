package cmd

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/tx"
	"github.com/tranvictor/jarvis/util"
)

var summaryMsigCmd = &cobra.Command{
	Use:   "summary",
	Short: "Print all txs confirmation and execution status of the multisig",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		msigAddress, err := getMsigContractFromParams(args)
		if err != nil {
			return
		}

		multisigContract, err := msig.NewMultisigContract(
			msigAddress,
			Network,
		)
		if err != nil {
			fmt.Printf("Couldn't interact with the contract: %s\n", err)
			return
		}

		noTxs, err := multisigContract.NOTransactions()
		if err != nil {
			fmt.Printf("Couldn't get number of transactions of the multisig: %s\n", err)
			return
		}
		fmt.Printf("Number of transactions: %d\n", noTxs)
		noExecuted := 0
		noRevoked := 0
		noConfirmed := 0
		noError := 0
		nonExecutedConfirmedIds := []int64{}
		for i := int64(0); i < noTxs; i++ {
			executed, err := multisigContract.IsExecuted(big.NewInt(i))
			if err != nil {
				fmt.Printf("%d. %s\n", i, err)
				noError++
				continue
			}
			if executed {
				fmt.Printf("%d. executed\n", i)
				noExecuted++
				noConfirmed++
				continue
			}
			confirmed, err := multisigContract.IsConfirmed(big.NewInt(i))
			if err != nil {
				fmt.Printf("%d. %s\n", i, err)
				noError++
				continue
			}
			if confirmed {
				fmt.Printf("%d. confirmed - not executed\n", i)
				nonExecutedConfirmedIds = append(nonExecutedConfirmedIds, i)
				noConfirmed++
				continue
			}
			fmt.Printf("%d. unconfirmed\n", i)
		}
		fmt.Printf("------------\n")
		fmt.Printf("Total executed txs: %d\n", noExecuted)
		fmt.Printf("Total confirmed but NOT executed txs: %d. Their ids are: %v\n", noConfirmed-noExecuted, nonExecutedConfirmedIds)
		fmt.Printf("Total revoked txs: %d\n", noRevoked)
		fmt.Printf("Total unconfirmed txs (including unknown txs): %d\n", int(noTxs)-noConfirmed-noRevoked)
	},
}

var transactionInfoMsigCmd = &cobra.Command{
	Use:   "info",
	Short: "Print all information about a multisig init",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		msigAddress, err := getMsigContractFromParams(args)
		if err != nil {
			return
		}

		multisigContract, err := msig.NewMultisigContract(
			msigAddress,
			Network,
		)
		if err != nil {
			fmt.Printf("Couldn't interact with the contract: %s\n", err)
			return
		}

		if len(args) < 2 {
			fmt.Printf("Jarvis: Please specify tx id in either hex or int format after the multisig address\n")
			return
		}
		idStr := strings.Trim(args[1], " ")
		var txid *big.Int
		if len(idStr) > 2 && idStr[0:2] == "0x" {
			txid = ethutils.HexToBig(idStr)
		} else {
			idInt, err := strconv.Atoi(idStr)
			if err != nil {
				fmt.Printf("Jarvis: Can't convert \"%s\" to int.\n", idStr)
				return
			}
			txid = big.NewInt(int64(idInt))
		}

		address, value, data, executed, confirmations, err := multisigContract.TransactionInfo(txid)
		if err != nil {
			fmt.Printf("Jarvis: Can't get tx info: %s\n", err)
			return
		}
		fmt.Printf("Sending: %f ETH to %s\n", ethutils.BigToFloat(value, 18), util.VerboseAddress(address))
		if len(data) != 0 {
			fmt.Printf("Calling on %s:\n", util.VerboseAddress(address))
			destAbi, err := util.GetABI(address, Network)
			if err != nil {
				fmt.Printf("Couldn't get abi of destination address: %s\n", err)
				return
			}
			tx.AnalyzeMethodCallAndPrint(destAbi, data, Network)
		}

		fmt.Printf("\nExecuted: %t\n", executed)
		fmt.Printf("Confirmations (among current owners):\n")
		for i, c := range confirmations {
			addrDesc, err := db.GetAddress(c)
			if err != nil {
				fmt.Printf("%d. %s (Unknown)\n", i+1, c)
			} else {
				fmt.Printf("%d. %s (%s)\n", i+1, c, addrDesc.Desc)
			}
		}
	},
}

var govInfoMsigCmd = &cobra.Command{
	Use:   "gov",
	Short: "Print goverance information of a Gnosis multisig",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		msigAddress, err := getMsigContractFromParams(args)
		if err != nil {
			return
		}

		multisigContract, err := msig.NewMultisigContract(
			msigAddress,
			Network,
		)
		if err != nil {
			fmt.Printf("Couldn't interact with the contract: %s\n", err)
			return
		}

		fmt.Printf("Owner list:\n")
		owners, err := multisigContract.Owners()
		if err != nil {
			fmt.Printf("Couldn't get owners of the multisig: %s\n", err)
			return
		}
		for i, owner := range owners {
			addrDesc, err := db.GetAddress(owner)
			if err != nil {
				fmt.Printf("%d. %s (Unknown)\n", i+1, owner)
			} else {
				fmt.Printf("%d. %s (%s)\n", i+1, owner, addrDesc.Desc)
			}
		}
		voteRequirement, err := multisigContract.VoteRequirement()
		if err != nil {
			fmt.Printf("Couldn't get vote requiremnts of the multisig: %s\n", err)
			return
		}
		fmt.Printf("Vote requirement: %d/%d\n", voteRequirement, len(owners))
		noTxs, err := multisigContract.NOTransactions()
		if err != nil {
			fmt.Printf("Couldn't get number of transactions of the multisig: %s\n", err)
			return
		}
		fmt.Printf("Number of transaction inited: %d\n", noTxs)
	},
}

func getMsigContractFromParams(args []string) (msigAddress string, err error) {
	if len(args) < 1 {
		fmt.Printf("Jarvis: Please specify multisig address\n")
		return "", fmt.Errorf("not enough params")
	}

	addrDesc, err := db.GetAddress(args[0])
	var msigName string
	if err != nil {
		msigName = "Unknown"
		addresses := util.ScanForAddresses(args[0])
		if len(addresses) == 0 {
			fmt.Printf("Couldn't find any address for \"%s\"", args[0])
			return "", fmt.Errorf("address not found")
		}
		msigAddress = addresses[0]
	} else {
		msigName = addrDesc.Desc
		msigAddress = addrDesc.Address
	}
	isGnosisMultisig, err := util.IsGnosisMultisig(msigAddress, Network)
	if err != nil {
		fmt.Printf("Checking failed, %s (%s) is not a contract or it doesn't have verified ABI on etherscan: %s\n", msigAddress, msigName, err)
		return "", err
	}
	if !isGnosisMultisig {
		fmt.Printf("Jarvis: %s (%s) is not a Gnosis multisig or not with a version I understand.\n", msigAddress, msigName)
		return "", fmt.Errorf("not gnosis multisig")
	}
	fmt.Printf("Multisig: %s (%s)\n", msigAddress, msigName)
	return msigAddress, nil
}

var msigCmd = &cobra.Command{
	Use:   "msig",
	Short: "Gnosis multisig operations",
	Long:  ``,
}

func init() {
	msigCmd.AddCommand(summaryMsigCmd)
	msigCmd.AddCommand(transactionInfoMsigCmd)
	msigCmd.AddCommand(govInfoMsigCmd)
	// TODO: add more commands to send or call other contracts
	rootCmd.AddCommand(msigCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// txCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// txCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
