package cmd

import (
	"github.com/spf13/cobra"

	"github.com/relaymesh/relaymesh/pkg/server"
)

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the webhook server",
		Long: "Run the webhook server that ingests provider webhooks, evaluates rules, and " +
			"publishes matching events to Relaybus.",
		Example: "  githook serve --config config.yaml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return server.RunConfig(configPath)
		},
	}
	return cmd
}
