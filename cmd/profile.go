package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/entryguard-io/cli/internal/api"
	"github.com/entryguard-io/cli/internal/config"
	"github.com/entryguard-io/cli/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage configuration profiles",
}

var profileAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		if _, exists := cfg.Profiles[name]; exists {
			return fmt.Errorf("profile %q already exists. Remove it first with: eg profile remove %s", name, name)
		}

		reader := bufio.NewReader(os.Stdin)

		fmt.Print("API Key: ")
		keyBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to read API key: %w", err)
		}
		apiKey := strings.TrimSpace(string(keyBytes))
		fmt.Println()

		if apiKey == "" {
			return fmt.Errorf("API key cannot be empty")
		}

		fmt.Print("API URL [https://app.entryguard.io/api/v1]: ")
		apiURL, _ := reader.ReadString('\n')
		apiURL = strings.TrimSpace(apiURL)
		if apiURL == "" {
			apiURL = "https://app.entryguard.io/api/v1"
		}

		output.Info("Validating API key...")
		client := api.NewClient(apiURL, apiKey)
		user, err := client.GetMe()
		if err != nil {
			return fmt.Errorf("API key validation failed: %w", err)
		}

		config.AddProfile(cfg, name, config.Profile{
			APIKey: apiKey,
			APIURL: apiURL,
		})

		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		output.Success("Profile %q added (org: %s, user: %s)", name, user.OrganizationName, user.Email)
		if cfg.DefaultProfile == name {
			output.Info("Set as default profile")
		}
		return nil
	},
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		if len(cfg.Profiles) == 0 {
			fmt.Println("No profiles configured. Run: eg profile add <name>")
			return nil
		}

		if output.Format == "json" {
			type profileEntry struct {
				Name    string `json:"name"`
				APIURL  string `json:"apiUrl"`
				Default bool   `json:"default"`
			}
			entries := make([]profileEntry, 0, len(cfg.Profiles))
			for name, p := range cfg.Profiles {
				entries = append(entries, profileEntry{
					Name:    name,
					APIURL:  p.APIURL,
					Default: name == cfg.DefaultProfile,
				})
			}
			output.PrintJSON(entries)
			return nil
		}

		var rows [][]string
		for name, p := range cfg.Profiles {
			marker := ""
			if name == cfg.DefaultProfile {
				marker = "*"
			}
			rows = append(rows, []string{marker, name, p.APIURL})
		}
		output.PrintTable([]string{"", "NAME", "API URL"}, rows)
		return nil
	},
}

var profileUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set the default profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		if _, ok := cfg.Profiles[name]; !ok {
			return fmt.Errorf("profile %q not found", name)
		}

		cfg.DefaultProfile = name
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		output.Success("Default profile set to %q", name)
		return nil
	},
}

var profileRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		if err := config.RemoveProfile(cfg, name); err != nil {
			return err
		}

		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		output.Success("Profile %q removed", name)
		return nil
	},
}

func init() {
	profileCmd.AddCommand(profileAddCmd)
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileUseCmd)
	profileCmd.AddCommand(profileRemoveCmd)
	rootCmd.AddCommand(profileCmd)
}
