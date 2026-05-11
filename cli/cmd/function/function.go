package funccmd

import "github.com/spf13/cobra"

func NewFunctionCommands() *cobra.Command {
	functionCmd := &cobra.Command{
		Use:     "function",
		Short:   "A command group for function management",
		Aliases: []string{"fn"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	functionCmd.AddCommand(
		newBuildFunctionCommand(),
		newDeployFunctionCommand(),
		newInitFunctionCommand(),
		newPushFunctionCommand(),
	)

	return functionCmd
}
