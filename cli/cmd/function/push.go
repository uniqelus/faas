package funccmd

import "github.com/spf13/cobra"

func newPushFunctionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push function docker image to local registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}
