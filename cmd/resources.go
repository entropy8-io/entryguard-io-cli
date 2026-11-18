package cmd

import (
	"fmt"

	"github.com/entryguard-io/cli/internal/output"
	"github.com/spf13/cobra"
)

var resourcesCmd = &cobra.Command{
	Use:   "resources",
	Short: "List the resources you can open a session on",
	Long: `Lists the active resources your roles and direct assignments give you, and
the longest session you may request. Use the names (or ids) with
"eg session start --resources".`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := getClient()
		if err != nil {
			return err
		}
		available, err := client.ListAvailableResources()
		if err != nil {
			return err
		}

		if output.Format == "json" {
			output.PrintJSON(available)
			return nil
		}

		var rows [][]string
		for _, r := range available.Resources {
			region := r.Region
			if region == "" {
				region = "-"
			}
			rows = append(rows, []string{r.Name, r.Provider, r.ResourceType, region, r.ID})
		}
		output.PrintTable([]string{"NAME", "PROVIDER", "TYPE", "REGION", "ID"}, rows)
		fmt.Printf("\nMax session length: %s\n", formatMinutes(available.MaxDurationMinutes))
		return nil
	},
}

func formatMinutes(m int) string {
	if m%60 == 0 {
		return fmt.Sprintf("%dh", m/60)
	}
	return fmt.Sprintf("%dh %dm", m/60, m%60)
}

func init() {
	rootCmd.AddCommand(resourcesCmd)
}
