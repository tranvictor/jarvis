package kyberdao

import (
	"github.com/spf13/cobra"
)

var KyberDAOCmd = &cobra.Command{
	Use:              "kyber-dao",
	Short:            "participate to Kyber DAO to get rewards",
	Long:             ``,
	TraverseChildren: true,
}

func init() {
	KyberDAOCmd.AddCommand(infoCmd)
	KyberDAOCmd.AddCommand(stakeCmd)
	KyberDAOCmd.AddCommand(withdrawCmd)
	KyberDAOCmd.AddCommand(claimCmd)
	KyberDAOCmd.AddCommand(voteCmd)
	KyberDAOCmd.AddCommand(createCamCmd)
}
