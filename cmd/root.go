package cmd

import (
	"fmt"
	"os"

	"github.com/entryguard-io/cli/internal/api"
	"github.com/entryguard-io/cli/internal/config"
	"github.com/entryguard-io/cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	profileFlag string
	outputFlag  string
)

var rootCmd = &cobra.Command{
	Use:   "eg",
	Short: "EntryGuard CLI — Dynamic IP whitelisting",
	Long:  "EntryGuard CLI tool for managing IP whitelisting sessions from the terminal.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if outputFlag != "" {
			output.Format = outputFlag
		}
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&profileFlag, "profile", "", "Profile to use (overrides default)")
	rootCmd.PersistentFlags().StringVar(&outputFlag, "output", "table", "Output format: table or json")
}

func SetVersion(v string) {
	rootCmd.Version = v
}

func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	return nil
}

func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return cfg, nil
}

// getClient authenticates with ENTRYGUARD_API_KEY (and optional ENTRYGUARD_API_URL)
// when set — the natural way for CI jobs and MCP clients to pass credentials —
// and falls back to the selected profile otherwise.
func getClient() (*api.Client, error) {
	if key := os.Getenv("ENTRYGUARD_API_KEY"); key != "" && profileFlag == "" {
		baseURL := os.Getenv("ENTRYGUARD_API_URL")
		if baseURL == "" {
			baseURL = defaultAPIURL
		}
		return api.NewClient(baseURL, key), nil
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	profile, err := config.GetProfile(cfg, profileFlag)
	if err != nil {
		if os.Getenv("ENTRYGUARD_API_KEY") == "" {
			return nil, fmt.Errorf("%w (or set ENTRYGUARD_API_KEY)", err)
		}
		return nil, err
	}
	return api.NewClient(profile.APIURL, profile.APIKey), nil
}

const defaultAPIURL = "https://app.entryguard.io/api/v1"

func getClientUnauthenticated() *api.Client {
	cfg, _ := loadConfig()
	baseURL := defaultAPIURL
	if cfg != nil {
		profile, err := config.GetProfile(cfg, profileFlag)
		if err == nil {
			baseURL = profile.APIURL
		}
	}
	return api.NewClient(baseURL, "")
}
