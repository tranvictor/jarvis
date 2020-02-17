package apps

import (
	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/cmd/apps/kyber"
)

var APPS []*cobra.Command

func init() {
	APPS = append(APPS, kyber.KyberCmd)
}
