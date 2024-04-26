package cmd

import (
	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/config"
)

func AddCommonFlagsToTransactionalCmds(c *cobra.Command) {
	c.PersistentFlags().
		Float64VarP(&config.GasPrice, "gasprice", "p", 0, "Gas price in gwei. If default value is used, we will use https://ethgasstation.info/ to get fast gas price. The gas price to be used in the tx is gas price + extra gas price")
	c.PersistentFlags().
		Float64VarP(&config.TipGas, "tipgas", "s", 0, "tip in gwei, will be use in dynamic fee tx, default value get from node.")
	c.PersistentFlags().
		Float64VarP(&config.ExtraGasPrice, "extraprice", "P", 0, "Extra gas price in gwei. The gas price to be used in the tx is gas price + extra gas price")
	c.PersistentFlags().
		Float64VarP(&config.ExtraTipGas, "extratip", "Q", 0, "Extra tip gas in gwei. The tip gas to be used in the tx is tip_gas_from_node + extra_tip_gas. This param will be ignored if dynamic tx is not possible.")
	c.PersistentFlags().
		Uint64VarP(&config.GasLimit, "gas", "g", 0, "Base gas limit for the tx. If default value is used, we will use ethereum nodes to estimate the gas limit. The gas limit to be used in the tx is gas limit + extra gas limit")
	c.PersistentFlags().
		Uint64VarP(&config.ExtraGasLimit, "extragas", "G", 250000, "Extra gas limit for the tx. The gas limit to be used in the tx is gas limit + extra gas limit")
	c.PersistentFlags().
		Uint64VarP(&config.Nonce, "nonce", "n", 0, "Nonce of the from account. If default value is used, we will use the next available nonce of from account")
	c.PersistentFlags().
		StringVarP(&config.From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	c.PersistentFlags().
		BoolVarP(&config.DontBroadcast, "dry", "d", false, "Will not broadcast the tx, only show signed tx.")
	c.PersistentFlags().
		BoolVarP(&config.DontWaitToBeMined, "no-wait", "F", false, "Will not wait the tx to be mined.")
	c.PersistentFlags().
		BoolVarP(&config.RetryBroadcast, "retry-broadcast", "r", false, "Retry broadcasting as soon as possible.")
	c.PersistentFlags().
		BoolVarP(&config.ForceLegacy, "legacy-tx", "L", false, "Force using legacy transaction")
}
