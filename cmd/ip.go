package cmd

import (
	"fmt"

	"github.com/entryguard-io/cli/internal/api"
	"github.com/entryguard-io/cli/internal/output"
	"github.com/spf13/cobra"
)

var ipCmd = &cobra.Command{
	Use:   "ip",
	Short: "Detect your public IP addresses",
	RunE: func(cmd *cobra.Command, args []string) error {
		ipv4, ipv6 := api.DetectIPs()

		if output.Format == "json" {
			result := map[string]string{}
			if ipv4 != "" {
				result["ipv4"] = ipv4
			}
			if ipv6 != "" {
				result["ipv6"] = ipv6
			}
			output.PrintJSON(result)
			return nil
		}

		if ipv4 != "" {
			fmt.Printf("IPv4: %s\n", ipv4)
		}
		if ipv6 != "" {
			fmt.Printf("IPv6: %s\n", ipv6)
		}
		if ipv4 == "" && ipv6 == "" {
			return fmt.Errorf("failed to detect any IP address")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(ipCmd)
}
