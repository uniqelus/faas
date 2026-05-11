package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	funccmd "github.com/uniqelus/faas/cli/cmd/function"
)

var rootCmd = &cobra.Command{
	Use:   "faas",
	Short: "Command line interface for Function as a Service platform management",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(funccmd.NewFunctionCommands())
}

func main() {
	executeCtx, executeCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer executeCancel()

	if executeErr := rootCmd.ExecuteContext(executeCtx); executeErr != nil {
		panic(executeErr)
	}
}
