package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	cloudv1connect "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1/cloudv1connect"
)

func newDriversCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drivers",
		Short: "Manage driver configs",
		Long:  "Manage Relaybus driver configs stored on the server.",
		Example: "  githook --endpoint http://localhost:8080 drivers list\n" +
			"  githook --endpoint http://localhost:8080 drivers create --name amqp --config-file amqp.yaml",
	}
	cmd.AddCommand(newDriversListCmd())
	cmd.AddCommand(newDriversGetCmd())
	cmd.AddCommand(newDriversCreateCmd())
	cmd.AddCommand(newDriversUpdateCmd())
	cmd.AddCommand(newDriversSetCmd())
	cmd.AddCommand(newDriversDeleteCmd())
	return cmd
}

func newDriversListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List driver configs",
		Example: "  githook --endpoint http://localhost:8080 drivers list",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewDriversServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.ListDriversRequest{})
			applyTenantHeader(req)
			resp, err := client.ListDrivers(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
}

func newDriversGetCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get a driver config",
		Example: "  githook --endpoint http://localhost:8080 drivers get --name amqp",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("name is required")
			}
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewDriversServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.GetDriverRequest{Name: strings.TrimSpace(name)})
			applyTenantHeader(req)
			resp, err := client.GetDriver(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", driverNameDescription)
	return cmd
}

func newDriversCreateCmd() *cobra.Command {
	return newDriversUpsertCmd(
		"create",
		"Create a driver config",
		"  githook --endpoint http://localhost:8080 drivers create --name amqp --config-file amqp.yaml",
	)
}

func newDriversUpdateCmd() *cobra.Command {
	return newDriversUpsertCmd(
		"update",
		"Update a driver config",
		"  githook --endpoint http://localhost:8080 drivers update --name amqp --config-file amqp.yaml",
	)
}

func newDriversSetCmd() *cobra.Command {
	cmd := newDriversUpsertCmd(
		"set",
		"Create or update a driver config",
		"  githook --endpoint http://localhost:8080 drivers set --name amqp --config-file amqp.yaml",
	)
	hideDeprecatedAlias(cmd, "use create or update")
	return cmd
}

func newDriversUpsertCmd(action, short, example string) *cobra.Command {
	var name string
	var configFile string
	var enabled bool
	cmd := &cobra.Command{
		Use:     action,
		Short:   short,
		Example: example,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNonEmpty("name", name); err != nil {
				return err
			}
			if strings.TrimSpace(configFile) == "" {
				return fmt.Errorf("config-file is required")
			}
			payload, err := loadConfigPayload(configFile)
			if err != nil {
				return err
			}
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewDriversServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.UpsertDriverRequest{
				Driver: &cloudv1.DriverRecord{
					Name:       strings.TrimSpace(name),
					ConfigJson: payload,
					Enabled:    enabled,
				},
			})
			applyTenantHeader(req)
			resp, err := client.UpsertDriver(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", driverNameDescription)
	cmd.Flags().StringVar(&configFile, "config-file", "", "Path to driver YAML config")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Enable this driver")
	return cmd
}

func newDriversDeleteCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete a driver config",
		Example: "  githook --endpoint http://localhost:8080 drivers delete --name amqp",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNonEmpty("name", name); err != nil {
				return err
			}
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewDriversServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.DeleteDriverRequest{Name: strings.TrimSpace(name)})
			applyTenantHeader(req)
			resp, err := client.DeleteDriver(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", driverNameDescription)
	return cmd
}
