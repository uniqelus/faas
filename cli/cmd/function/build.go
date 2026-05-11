package funccmd

import "github.com/spf13/cobra"

func newBuildFunctionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Local build of function docker image",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}
