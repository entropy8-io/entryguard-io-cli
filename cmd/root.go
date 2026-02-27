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

func getClient() (*api.Client, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	profile, err := config.GetProfile(cfg, profileFlag)
	if err != nil {
		return nil, err
	}
	return api.NewClient(profile.APIURL, profile.APIKey), nil
}

func getClientUnauthenticated() *api.Client {
	cfg, _ := loadConfig()
	baseURL := "https://app.entryguard.io/api/v1"
	if cfg != nil {
		profile, err := config.GetProfile(cfg, profileFlag)
		if err == nil {
			baseURL = profile.APIURL
		}
	}
	return api.NewClient(baseURL, "")
}
