package cmd

import (
	"context"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	cloudv1connect "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1/cloudv1connect"
)

func newNamespacesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "namespaces",
		Short: "Manage git namespaces",
		Long:  "List, update, and toggle webhooks for provider namespaces (repos/projects).",
		Example: "  githook --endpoint http://localhost:8080 namespaces list --provider github\n" +
			"  githook --endpoint http://localhost:8080 namespaces update --provider gitlab",
	}
	cmd.AddCommand(newNamespacesListCmd())
	cmd.AddCommand(newNamespacesUpdateCmd())
	cmd.AddCommand(newNamespacesSyncCmd())
	cmd.AddCommand(newNamespacesWebhookCmd())
	return cmd
}

func newNamespacesListCmd() *cobra.Command {
	var stateID string
	var provider string
	var owner string
	var repo string
	var fullName string
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List namespaces",
		Example: "  githook --endpoint http://localhost:8080 namespaces list --provider github",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewNamespacesServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.ListNamespacesRequest{
				StateId:  stateID,
				Provider: strings.TrimSpace(provider),
				Owner:    strings.TrimSpace(owner),
				Repo:     strings.TrimSpace(repo),
				FullName: strings.TrimSpace(fullName),
			})
			applyTenantHeader(req)
			resp, err := client.ListNamespaces(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&stateID, "state-id", "", stateIDDescription)
	cmd.Flags().StringVar(&provider, "provider", "", providerFlagDescription)
	cmd.Flags().StringVar(&owner, "owner", "", "Owner filter")
	cmd.Flags().StringVar(&repo, "repo", "", "Repo filter")
	cmd.Flags().StringVar(&fullName, "full-name", "", "Full name filter")
	return cmd
}

func newNamespacesUpdateCmd() *cobra.Command {
	var stateID string
	var provider string
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Update namespaces from the provider",
		Example: "  githook --endpoint http://localhost:8080 namespaces update --provider gitlab",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNonEmpty("provider", provider); err != nil {
				return err
			}
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewNamespacesServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.SyncNamespacesRequest{
				StateId:  stateID,
				Provider: provider,
			})
			applyTenantHeader(req)
			resp, err := client.SyncNamespaces(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&stateID, "state-id", "", stateIDDescription)
	cmd.Flags().StringVar(&provider, "provider", "", providerFlagDescription)
	return cmd
}

func newNamespacesSyncCmd() *cobra.Command {
	cmd := newNamespacesUpdateCmd()
	cmd.Use = "sync"
	cmd.Short = "Sync namespaces from the provider"
	cmd.Example = "  githook --endpoint http://localhost:8080 namespaces sync --provider gitlab"
	hideDeprecatedAlias(cmd, "use update")
	return cmd
}

func newNamespacesWebhookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "webhook",
		Short:   "Manage namespace webhook state",
		Example: "  githook --endpoint http://localhost:8080 namespaces webhook get --provider gitlab --repo-id <repo-id>",
	}
	cmd.AddCommand(newNamespacesWebhookGetCmd())
	cmd.AddCommand(newNamespacesWebhookUpdateCmd())
	cmd.AddCommand(newNamespacesWebhookSetCmdAlias())
	return cmd
}

func newNamespacesWebhookGetCmd() *cobra.Command {
	var stateID string
	var provider string
	var repoID string
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get namespace webhook state",
		Example: "  githook --endpoint http://localhost:8080 namespaces webhook get --provider gitlab --repo-id <repo-id>",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNonEmpty("provider", provider); err != nil {
				return err
			}
			if err := requireNonEmpty("repo-id", repoID); err != nil {
				return err
			}
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewNamespacesServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.GetNamespaceWebhookRequest{
				StateId:  stateID,
				Provider: provider,
				RepoId:   repoID,
			})
			applyTenantHeader(req)
			resp, err := client.GetNamespaceWebhook(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&stateID, "state-id", "", stateIDDescription)
	cmd.Flags().StringVar(&provider, "provider", "", providerFlagDescription)
	cmd.Flags().StringVar(&repoID, "repo-id", "", "Repo ID")
	return cmd
}

func newNamespacesWebhookUpdateCmd() *cobra.Command {
	var stateID string
	var provider string
	var repoID string
	var enabled bool
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Enable or disable a namespace webhook",
		Example: "  githook --endpoint http://localhost:8080 namespaces webhook update --provider gitlab --repo-id <repo-id> --enabled",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNonEmpty("provider", provider); err != nil {
				return err
			}
			if err := requireNonEmpty("repo-id", repoID); err != nil {
				return err
			}
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewNamespacesServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.SetNamespaceWebhookRequest{
				StateId:  stateID,
				Provider: provider,
				RepoId:   repoID,
				Enabled:  enabled,
			})
			applyTenantHeader(req)
			resp, err := client.SetNamespaceWebhook(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&stateID, "state-id", "", stateIDDescription)
	cmd.Flags().StringVar(&provider, "provider", "", providerFlagDescription)
	cmd.Flags().StringVar(&repoID, "repo-id", "", "Repo ID")
	cmd.Flags().BoolVar(&enabled, "enabled", false, "Enable or disable webhook")
	return cmd
}

func newNamespacesWebhookSetCmdAlias() *cobra.Command {
	cmd := newNamespacesWebhookUpdateCmd()
	cmd.Use = "set"
	cmd.Short = "Enable or disable a namespace webhook"
	cmd.Example = "  githook --endpoint http://localhost:8080 namespaces webhook set --provider gitlab --repo-id <repo-id> --enabled"
	hideDeprecatedAlias(cmd, "use update")
	return cmd
}
