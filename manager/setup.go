package manager

import (
	"github.com/bushubdegefu/blue-proxy.com/helper"
	"github.com/spf13/cobra"
)

var (
	targetsTemplate = &cobra.Command{
		Use:   "targets",
		Short: "generate targets json file",
		Long: `Generate targets json file that is used to define the target endpoints to proxy and load balance too.
		Note that the actual file should have the same structure as the generated file for the app to work properly.`,
		Run: func(cmd *cobra.Command, args []string) {
			templplatecmd()
		},
	}
	sampleEnv = &cobra.Command{
		Use:   "env",
		Short: "generate basic environment file used to run the echo based proxy server",
		Long:  `Generate .env file parameters required to run the echo based proxy server. This file contains the necessary environment variables for the proxy server to function properly.	`,
		Run: func(cmd *cobra.Command, args []string) {
			templplateEnv()
		},
	}
)

func templplatecmd() {
	helper.TargetsFrame()
}

func templplateEnv() {
	helper.NormalEnviromentFrame()
	helper.EnviromentFrame()
}

func init() {
	goFrame.AddCommand(targetsTemplate)
	goFrame.AddCommand(sampleEnv)

}
