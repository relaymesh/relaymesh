package cmd

import "github.com/spf13/cobra"

// NewRootCmd returns the Cobra entrypoint for the CLI/server.
func NewRootCmd() *cobra.Command {
	apiBaseURL = ""
	configPath = "config.yaml"
	root := &cobra.Command{
		Use:   "githook",
		Short: "Webhook router + worker SDK for Git providers",
		Long: "github.com/relaymesh/relaymeshs routes GitHub/GitLab/Bitbucket webhooks to Relaybus topics and provides a worker SDK " +
			"for processing events with provider-aware clients.",
		Example: "  githook serve --config config.yaml\n" +
			"  githook --endpoint http://localhost:8080 installations list --provider github\n" +
			"  githook --endpoint http://localhost:8080 namespaces update --provider gitlab",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.PersistentFlags().StringVar(&apiBaseURL, "endpoint", apiBaseURL, "Connect RPC endpoint base URL")
	root.PersistentFlags().StringVar(&configPath, "config", configPath, "Path to config file")
	root.PersistentFlags().StringVar(&tenantID, "tenant-id", tenantID, "Tenant ID to scope API requests (default \"default\")")
	root.AddCommand(newServeCmd())
	root.AddCommand(newInstallationsCmd())
	root.AddCommand(newNamespacesCmd())
	root.AddCommand(newRulesCmd())
	root.AddCommand(newProvidersCmd())
	root.AddCommand(newDriversCmd())
	root.AddCommand(newAuthCmd())
	root.AddCommand(newInitCmd())
	return root
}

var apiBaseURL string
var configPath string

// tenantID is set via the persistent --tenant-id flag defined in cmd/tenant.go.
