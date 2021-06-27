package apps

import (
	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/cmd/apps/kyber_dao_v1"
	"github.com/tranvictor/jarvis/cmd/apps/kyber_fpr"
)

var APPS []*cobra.Command

func init() {
	APPS = append(APPS, kyberdao.KyberDAOCmd)
	APPS = append(APPS, kyberfpr.KyberFPRCmd)
}
