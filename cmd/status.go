package cmd

import (
	"fmt"

	"github.com/entryguard-io/cli/internal/api"
	"github.com/entryguard-io/cli/internal/output"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show profile info, active sessions, and detected IP",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := getClient()
		if err != nil {
			return err
		}

		type statusResult struct {
			User     *api.UserInfo
			Sessions []api.Session
			IPv4     string
			IPv6     string
			Errors   []error
		}

		result := &statusResult{}

		// Fetch all in sequence (simple, avoids goroutine complexity for a CLI)
		user, err := client.GetMe()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("profile: %w", err))
		}
		result.User = user

		sessions, err := client.ListSessions()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("sessions: %w", err))
		}
		result.Sessions = sessions

		result.IPv4, result.IPv6 = api.DetectIPs()

		if output.Format == "json" {
			ips := map[string]string{}
			if result.IPv4 != "" {
				ips["ipv4"] = result.IPv4
			}
			if result.IPv6 != "" {
				ips["ipv6"] = result.IPv6
			}
			output.PrintJSON(map[string]any{
				"user":     result.User,
				"sessions": result.Sessions,
				"ip":       ips,
			})
			return nil
		}

		bold := color.New(color.Bold).SprintFunc()

		// Profile
		fmt.Println(bold("Profile"))
		if result.User != nil {
			fmt.Printf("  Organization: %s\n", result.User.OrganizationName)
			fmt.Printf("  User:         %s (%s)\n", result.User.Name, result.User.Email)
			fmt.Printf("  Tier:         %s\n", result.User.SubscriptionTier)
			role := "User"
			if result.User.IsOrgAdmin {
				role = "Admin"
			}
			fmt.Printf("  Role:         %s\n", role)
		} else {
			fmt.Println("  (unavailable)")
		}
		fmt.Println()

		// IP
		fmt.Println(bold("Detected IP"))
		if result.IPv4 != "" {
			fmt.Printf("  IPv4: %s\n", result.IPv4)
		}
		if result.IPv6 != "" {
			fmt.Printf("  IPv6: %s\n", result.IPv6)
		}
		if result.IPv4 == "" && result.IPv6 == "" {
			fmt.Println("  (unavailable)")
		}
		fmt.Println()

		// Active Sessions
		fmt.Println(bold("Active Sessions"))
		var active []api.Session
		for _, s := range result.Sessions {
			if s.Status == "ACTIVE" || s.Status == "PARTIAL" || s.Status == "PENDING" {
				active = append(active, s)
			}
		}

		if len(active) == 0 {
			fmt.Println("  No active sessions")
		} else {
			var rows [][]string
			for _, s := range active {
				ip := s.Ipv4Address
				if ip == "" {
					ip = s.Ipv6Address
				}
				rows = append(rows, []string{
					s.ID[:8],
					output.StatusColor(s.Status),
					ip,
					output.FormatDuration(s.ExpiresAt),
				})
			}
			output.PrintTable([]string{"ID", "STATUS", "IP", "REMAINING"}, rows)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
