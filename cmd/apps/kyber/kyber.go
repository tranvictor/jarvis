package kyber

import (
	"github.com/spf13/cobra"
)

var KyberCmd = &cobra.Command{
	Use:              "kyber",
	Short:            "Kyber is the best protocol that provides the best liquidity in the world",
	Long:             ``,
	TraverseChildren: true,
}

func init() {
	KyberCmd.AddCommand(daoCmd)
}
