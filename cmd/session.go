package cmd

import (
	"fmt"

	"github.com/entryguard-io/cli/internal/api"
	"github.com/entryguard-io/cli/internal/output"
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage IP whitelisting sessions",
}

var (
	sessionDuration int
	sessionIPv4     string
	sessionIPv6     string
	extendHours     int
)

var sessionStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new IP whitelisting session",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := getClient()
		if err != nil {
			return err
		}

		req := &api.StartSessionRequest{}
		if sessionDuration > 0 {
			req.DurationHours = &sessionDuration
		}
		if sessionIPv4 != "" {
			req.Ipv4Address = sessionIPv4
		}
		if sessionIPv6 != "" {
			req.Ipv6Address = sessionIPv6
		}

		output.Info("Starting session...")
		session, err := client.StartSession(req)
		if err != nil {
			return err
		}

		if output.Format == "json" {
			output.PrintJSON(session)
			return nil
		}

		output.Success("Session started")
		printSessionSummary(session)
		return nil
	},
}

var sessionStopCmd = &cobra.Command{
	Use:   "stop [id]",
	Short: "Stop a session (defaults to most recent active)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := getClient()
		if err != nil {
			return err
		}

		var sessionID string
		if len(args) > 0 {
			sessionID = args[0]
		} else {
			sessions, err := client.ListSessions()
			if err != nil {
				return err
			}
			for _, s := range sessions {
				if s.Status == "ACTIVE" || s.Status == "PARTIAL" {
					sessionID = s.ID
					break
				}
			}
			if sessionID == "" {
				return fmt.Errorf("no active session found")
			}
		}

		output.Info("Stopping session %s...", sessionID[:8])
		session, err := client.StopSession(sessionID)
		if err != nil {
			return err
		}

		if output.Format == "json" {
			output.PrintJSON(session)
			return nil
		}

		output.Success("Session stopped")
		return nil
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := getClient()
		if err != nil {
			return err
		}

		sessions, err := client.ListSessions()
		if err != nil {
			return err
		}

		if output.Format == "json" {
			output.PrintJSON(sessions)
			return nil
		}

		var rows [][]string
		for _, s := range sessions {
			ip := s.Ipv4Address
			if ip == "" {
				ip = s.Ipv6Address
			}
			if s.Ipv4Address != "" && s.Ipv6Address != "" {
				ip = s.Ipv4Address + ", " + s.Ipv6Address
			}
			rows = append(rows, []string{
				s.ID[:8],
				output.StatusColor(s.Status),
				ip,
				output.FormatTime(s.StartedAt),
				output.FormatDuration(s.ExpiresAt),
			})
		}
		output.PrintTable([]string{"ID", "STATUS", "IP", "STARTED", "REMAINING"}, rows)
		return nil
	},
}

var sessionGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get session details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := getClient()
		if err != nil {
			return err
		}

		session, err := client.GetSession(args[0])
		if err != nil {
			return err
		}

		if output.Format == "json" {
			output.PrintJSON(session)
			return nil
		}

		printSessionDetail(session)
		return nil
	},
}

var sessionExtendCmd = &cobra.Command{
	Use:   "extend <id>",
	Short: "Extend an active session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if extendHours <= 0 {
			return fmt.Errorf("--hours is required and must be positive")
		}

		client, err := getClient()
		if err != nil {
			return err
		}

		output.Info("Extending session %s by %d hours...", args[0][:8], extendHours)
		session, err := client.ExtendSession(args[0], extendHours)
		if err != nil {
			return err
		}

		if output.Format == "json" {
			output.PrintJSON(session)
			return nil
		}

		output.Success("Session extended — new expiry: %s (%s remaining)",
			output.FormatTime(session.ExpiresAt), output.FormatDuration(session.ExpiresAt))
		return nil
	},
}

func printSessionSummary(s *api.Session) {
	fmt.Printf("  ID:        %s\n", s.ID)
	fmt.Printf("  Status:    %s\n", output.StatusColor(s.Status))
	if s.Ipv4Address != "" {
		fmt.Printf("  IPv4:      %s\n", s.Ipv4Address)
	}
	if s.Ipv6Address != "" {
		fmt.Printf("  IPv6:      %s\n", s.Ipv6Address)
	}
	fmt.Printf("  Expires:   %s (%s remaining)\n", output.FormatTime(s.ExpiresAt), output.FormatDuration(s.ExpiresAt))
	if len(s.ResourceIps) > 0 {
		fmt.Printf("  Resources: %d\n", len(s.ResourceIps))
	}
}

func printSessionDetail(s *api.Session) {
	fmt.Printf("Session %s\n", s.ID)
	fmt.Printf("  Status:    %s\n", output.StatusColor(s.Status))
	fmt.Printf("  User:      %s (%s)\n", s.UserName, s.UserEmail)
	if s.Ipv4Address != "" {
		fmt.Printf("  IPv4:      %s\n", s.Ipv4Address)
	}
	if s.Ipv6Address != "" {
		fmt.Printf("  IPv6:      %s\n", s.Ipv6Address)
	}
	fmt.Printf("  Started:   %s\n", output.FormatTime(s.StartedAt))
	fmt.Printf("  Expires:   %s (%s remaining)\n", output.FormatTime(s.ExpiresAt), output.FormatDuration(s.ExpiresAt))
	if s.EndedAt != "" {
		fmt.Printf("  Ended:     %s (%s)\n", output.FormatTime(s.EndedAt), s.EndedReason)
	}

	if len(s.ResourceIps) > 0 {
		fmt.Println()
		var rows [][]string
		for _, r := range s.ResourceIps {
			rows = append(rows, []string{
				r.ResourceName,
				fmt.Sprintf("IPv%d", r.IpVersion),
				r.IpAddress,
				output.StatusColor(r.Status),
				output.FormatTime(r.AppliedAt),
			})
		}
		output.PrintTable([]string{"RESOURCE", "VERSION", "IP", "STATUS", "APPLIED"}, rows)
	}
}

func init() {
	sessionStartCmd.Flags().IntVar(&sessionDuration, "duration", 0, "Session duration in hours")
	sessionStartCmd.Flags().StringVar(&sessionIPv4, "ipv4", "", "IPv4 address to whitelist")
	sessionStartCmd.Flags().StringVar(&sessionIPv6, "ipv6", "", "IPv6 address to whitelist")

	sessionExtendCmd.Flags().IntVar(&extendHours, "hours", 0, "Hours to extend")
	sessionExtendCmd.MarkFlagRequired("hours")

	sessionCmd.AddCommand(sessionStartCmd)
	sessionCmd.AddCommand(sessionStopCmd)
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionGetCmd)
	sessionCmd.AddCommand(sessionExtendCmd)
	rootCmd.AddCommand(sessionCmd)
}
