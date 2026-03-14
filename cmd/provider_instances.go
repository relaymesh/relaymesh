package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	cloudv1connect "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1/cloudv1connect"
)

func newProvidersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "providers",
		Short: "Manage providers",
		Long:  "Manage per-tenant provider instances stored on the server.",
		Example: "  githook --endpoint http://localhost:8080 providers list\n" +
			"  githook --endpoint http://localhost:8080 providers list --provider github\n" +
			"  githook --endpoint http://localhost:8080 providers create --provider github --config-file github.yaml",
	}
	cmd.AddCommand(newProviderInstancesListCmd())
	cmd.AddCommand(newProviderInstancesGetCmd())
	cmd.AddCommand(newProviderInstancesCreateCmd())
	cmd.AddCommand(newProviderInstancesUpdateCmd())
	cmd.AddCommand(newProviderInstancesSetCmd())
	cmd.AddCommand(newProviderInstancesDeleteCmd())
	return cmd
}

func newProviderInstancesListCmd() *cobra.Command {
	var provider string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List provider instances",
		Example: "  githook --endpoint http://localhost:8080 providers list\n" +
			"  githook --endpoint http://localhost:8080 providers list --provider github",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewProvidersServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.ListProvidersRequest{Provider: strings.TrimSpace(provider)})
			applyTenantHeader(req)
			resp, err := client.ListProviders(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", providerFlagDescription)
	return cmd
}

func newProviderInstancesGetCmd() *cobra.Command {
	var provider string
	var hash string
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get a provider instance",
		Example: "  githook --endpoint http://localhost:8080 providers get --provider github --hash <instance-hash>",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNonEmpty("provider", provider); err != nil {
				return err
			}
			if err := requireNonEmpty("hash", hash); err != nil {
				return err
			}
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewProvidersServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.GetProviderRequest{
				Provider: strings.TrimSpace(provider),
				Hash:     strings.TrimSpace(hash),
			})
			applyTenantHeader(req)
			resp, err := client.GetProvider(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", providerFlagDescription)
	cmd.Flags().StringVar(&hash, "hash", "", "Instance hash")
	return cmd
}

func newProviderInstancesCreateCmd() *cobra.Command {
	return newProviderInstancesUpsertCmd(
		"create",
		"Create a provider instance",
		"  githook --endpoint http://localhost:8080 providers create --provider github --config-file github.yaml\n"+
			"  # With redirect_base_url and private_key_path in the config file:\n"+
			"  # github.yaml:\n"+
			"  # redirect_base_url: https://app.example.com/oauth/complete\n"+
			"  # app:\n"+
			"  #   app_id: 12345\n"+
			"  #   private_key_path: ./github.pem\n"+
			"  # oauth:\n"+
			"  #   client_id: your-client-id\n"+
			"  #   client_secret: your-client-secret",
	)
}

func newProviderInstancesUpdateCmd() *cobra.Command {
	return newProviderInstancesUpsertCmd(
		"update",
		"Update a provider instance",
		"  githook --endpoint http://localhost:8080 providers update --provider github --config-file github.yaml",
	)
}

func newProviderInstancesSetCmd() *cobra.Command {
	cmd := newProviderInstancesUpsertCmd(
		"set",
		"Create a provider instance",
		"  githook --endpoint http://localhost:8080 providers set --provider github --config-file github.yaml",
	)
	hideDeprecatedAlias(cmd, "use create or update")
	return cmd
}

func newProviderInstancesUpsertCmd(action, short, example string) *cobra.Command {
	var provider string
	var configFile string
	var enabled bool
	var redirectBaseURL string
	cmd := &cobra.Command{
		Use:     action,
		Short:   short,
		Example: example,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNonEmpty("provider", provider); err != nil {
				return err
			}
			var err error
			if strings.TrimSpace(configFile) == "" {
				return fmt.Errorf("config-file is required")
			}
			payload, err := loadConfigPayload(configFile)
			if err != nil {
				return err
			}
			payload, redirectURL, err := extractAndRemoveRedirectBaseURL(payload)
			if err != nil {
				return fmt.Errorf("failed to process config: %w", err)
			}
			payload, err = injectPrivateKeyPem(payload, "", configFile)
			if err != nil {
				return fmt.Errorf("failed to inject private key: %w", err)
			}
			// CLI flag overrides config file value
			if strings.TrimSpace(redirectBaseURL) != "" {
				redirectURL = strings.TrimSpace(redirectBaseURL)
			}
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewProvidersServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.UpsertProviderRequest{
				Provider: &cloudv1.ProviderRecord{
					Provider:        strings.TrimSpace(provider),
					ConfigJson:      payload,
					Enabled:         enabled,
					RedirectBaseUrl: redirectURL,
				},
			})
			applyTenantHeader(req)
			resp, err := client.UpsertProvider(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", providerFlagDescription)
	cmd.Flags().StringVar(&configFile, "config-file", "", "Path to provider YAML config (can include redirect_base_url)")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Enable this provider instance")
	cmd.Flags().StringVar(&redirectBaseURL, "redirect-base-url", "", "Post-OAuth redirect URL (overrides config file)")
	return cmd
}

// extractAndRemoveRedirectBaseURL extracts redirect_base_url from the config JSON
// and removes it from the payload (since it's stored separately in the database).
func extractAndRemoveRedirectBaseURL(payload string) (string, string, error) {
	var target map[string]any
	if err := json.Unmarshal([]byte(payload), &target); err != nil {
		return payload, "", nil // Return original if not valid JSON
	}
	redirectURL := ""
	if val, ok := target["redirect_base_url"].(string); ok {
		redirectURL = strings.TrimSpace(val)
		delete(target, "redirect_base_url")
	}
	out, err := json.Marshal(target)
	if err != nil {
		return payload, redirectURL, err
	}
	return string(out), redirectURL, nil
}

func injectPrivateKeyPem(payload, keyPath, configFile string) (string, error) {
	var target map[string]any
	if err := json.Unmarshal([]byte(payload), &target); err != nil {
		return "", err
	}
	app, _ := target["app"].(map[string]any)
	if app == nil {
		app = make(map[string]any)
	}
	var pem string
	var keyPathFromConfig string
	if keyPath != "" {
		keyPath = resolveConfigPath(configFile, keyPath)
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return "", err
		}
		pem = string(data)
	}
	if strings.TrimSpace(pem) == "" {
		if existing, ok := app["private_key_pem"].(string); ok && strings.TrimSpace(existing) != "" {
			pem = existing
		}
	}
	if strings.TrimSpace(pem) == "" {
		if pathVal, ok := app["private_key_path"].(string); ok && strings.TrimSpace(pathVal) != "" {
			keyPathFromConfig = pathVal
			pathVal = resolveConfigPath(configFile, pathVal)
			data, err := os.ReadFile(pathVal)
			if err != nil {
				return "", err
			}
			pem = string(data)
		}
	}
	if strings.TrimSpace(pem) != "" {
		app["private_key_pem"] = pem
		if strings.TrimSpace(keyPathFromConfig) != "" {
			delete(app, "private_key_path")
		}
	}
	target["app"] = app
	out, err := json.Marshal(target)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func resolveConfigPath(configFile, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) || configFile == "" {
		return value
	}
	dir := filepath.Dir(strings.TrimSpace(configFile))
	if dir == "" || dir == "." {
		return value
	}
	return filepath.Join(dir, value)
}

func newProviderInstancesDeleteCmd() *cobra.Command {
	var provider string
	var hash string
	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete a provider instance",
		Example: "  githook --endpoint http://localhost:8080 providers delete --provider github --hash <instance-hash>",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNonEmpty("provider", provider); err != nil {
				return err
			}
			if err := requireNonEmpty("hash", hash); err != nil {
				return err
			}
			opts, err := connectClientOptions()
			if err != nil {
				return err
			}
			client := cloudv1connect.NewProvidersServiceClient(http.DefaultClient, apiBaseURL, opts...)
			req := connect.NewRequest(&cloudv1.DeleteProviderRequest{
				Provider: strings.TrimSpace(provider),
				Hash:     strings.TrimSpace(hash),
			})
			applyTenantHeader(req)
			resp, err := client.DeleteProvider(context.Background(), req)
			if err != nil {
				return err
			}
			return printJSON(resp.Msg)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", providerFlagDescription)
	cmd.Flags().StringVar(&hash, "hash", "", "Instance hash")
	return cmd
}
