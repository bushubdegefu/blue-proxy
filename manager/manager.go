package manager

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	goFrame = &cobra.Command{
		Use:           "Blue Proxy",
		Short:         "Blue Proxy – Blue Proxy simple Echo reverse proxy",
		Long:          "Blue Proxy – Blue Proxy simple Echo reverse proxy with support for multiple backends and load balancing(roudnd-robbin) and otel and tls support",
		Version:       "0.1.0",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
)

func Execute() {
	if err := goFrame.Execute(); err != nil {

		fmt.Println(err)
		os.Exit(1)
	}
}
