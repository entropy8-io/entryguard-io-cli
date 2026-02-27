package cmd

import (
	"fmt"

	"github.com/entryguard-io/cli/internal/output"
	"github.com/spf13/cobra"
)

var ipCmd = &cobra.Command{
	Use:   "ip",
	Short: "Detect your public IP address",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClientUnauthenticated()

		ip, err := client.DetectIP()
		if err != nil {
			return fmt.Errorf("failed to detect IP: %w", err)
		}

		if output.Format == "json" {
			output.PrintJSON(ip)
			return nil
		}

		fmt.Printf("%s (IPv%d)\n", ip.IP, ip.Version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(ipCmd)
}
