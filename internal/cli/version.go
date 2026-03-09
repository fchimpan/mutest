package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of mutest",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("mutest version", version)
		},
	}
}
