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

func newInstallationsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "installations",
		Short: "Inspect stored installations",
		Long:  "Query installation records resolved from provider OAuth/App flows.",
		Example: "  githook --endpoint http://localhost:8080 installations list --provider github\n" +
			"  githook --endpoint http://localhost:8080 installations list --provider github --state-id <state-id>\n" +
			"  githook --endpoint http://localhost:8080 installations get --provider github --installation-id <id>",
	}
	cmd.AddCommand(newInstallationsListCmd())
	cmd.AddCommand(newInstallationsGetCmd())
	return cmd
}

func newInstallationsListCmd() *cobra.Command {
	var stateID string
	var provider string
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List installations",
		Example: "  githook --endpoint http://localhost:8080 installations list --provider github",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewInstallationsServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.ListInstallationsRequest{
				StateId:  stateID,
				Provider: strings.TrimSpace(provider),
			})
			applyTenantHeader(req)
			resp, err := client.ListInstallations(context.Background(), req)
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

func newInstallationsGetCmd() *cobra.Command {
	var provider string
	var installationID string
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get installation by provider and installation ID",
		Example: "  githook --endpoint http://localhost:8080 installations get --provider github --installation-id <id>",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNonEmpty("provider", provider); err != nil {
				return err
			}
			if err := requireNonEmpty("installation-id", installationID); err != nil {
				return err
			}
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewInstallationsServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.GetInstallationByIDRequest{
				Provider:       provider,
				InstallationId: installationID,
			})
			applyTenantHeader(req)
			resp, err := client.GetInstallationByID(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", providerFlagDescription)
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Installation ID")
	return cmd
}
