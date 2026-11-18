package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/entryguard-io/cli/internal/api"
	"github.com/entryguard-io/cli/internal/output"
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage IP whitelisting sessions",
}

var (
	sessionDuration  string
	sessionIPv4      string
	sessionIPv6      string
	sessionResources []string
	sessionLabel     string
	sessionWait      bool
	sessionWaitFor   time.Duration
	extendHours      int
)

var sessionStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new IP whitelisting session",
	Long: `Start a session that whitelists your public IP on your cloud resources.

Without --resources the session opens every resource you can access (one such
session per IP). With --resources it opens only those, by name or id; scoped
sessions may overlap on an IP and each job stops its own. Run "eg resources"
to see what you can open.`,
	Example: `  eg session start
  eg session start --duration 15m --resources staging-db-sg --label "deploy 1234" --wait
  eg session start --duration 4 --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		duration, err := api.ParseSessionDuration(sessionDuration)
		if err != nil {
			return err
		}
		client, err := getClient()
		if err != nil {
			return err
		}

		req := &api.StartSessionRequest{Label: sessionLabel}
		duration.Apply(req)
		if len(sessionResources) > 0 {
			req.Resources = sessionResources
		}
		if sessionIPv4 != "" {
			req.Ipv4Address = sessionIPv4
		}
		if sessionIPv6 != "" {
			req.Ipv6Address = sessionIPv6
		}

		// Auto-detect IPs when no flags provided
		if sessionIPv4 == "" && sessionIPv6 == "" {
			output.Info("Detecting IP addresses...")
			ipv4, ipv6 := api.DetectIPs()
			req.Ipv4Address = ipv4
			req.Ipv6Address = ipv6
			if ipv4 != "" && ipv6 != "" {
				output.Info("Detected IPv4: %s, IPv6: %s", ipv4, ipv6)
			} else if ipv4 != "" {
				output.Info("Detected IPv4: %s", ipv4)
			} else if ipv6 != "" {
				output.Info("Detected IPv6: %s", ipv6)
			} else {
				output.Info("Client-side detection failed, using server-side detection")
			}
		}

		output.Info("Starting session...")
		session, err := client.StartSession(req)
		if err != nil {
			return err
		}

		if sessionWait {
			output.Info("Waiting for IP rules to apply...")
			settled, waitErr := client.WaitForSession(session.ID, sessionWaitFor, 2*time.Second)
			if settled != nil {
				session = settled
			}
			if waitErr != nil {
				if output.Format == "json" {
					output.PrintJSON(session)
				}
				return waitErr
			}
		}

		if output.Format == "json" {
			output.PrintJSON(session)
			return nil
		}

		output.Success("Session started")
		printSessionSummary(session)
		if sessionWait && session.Status != "ACTIVE" {
			return fmt.Errorf("session ended up %s; see the per-resource errors with: eg session get %s", session.Status, session.ID[:8])
		}
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
			resolved, err := resolveSessionID(client, args[0])
			if err != nil {
				return err
			}
			sessionID = resolved
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

		sessionID, err := resolveSessionID(client, args[0])
		if err != nil {
			return err
		}

		session, err := client.GetSession(sessionID)
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

		sessionID, err := resolveSessionID(client, args[0])
		if err != nil {
			return err
		}

		output.Info("Extending session %s by %d hours...", sessionID[:8], extendHours)
		session, err := client.ExtendSession(sessionID, extendHours)
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

// resolveSessionID resolves a full or prefix session ID to the full UUID.
// If the input is already a full UUID (36 chars), it's returned as-is.
// Otherwise, it fetches the session list and matches by prefix.
func resolveSessionID(client *api.Client, input string) (string, error) {
	if len(input) >= 36 {
		return input, nil
	}

	sessions, err := client.ListSessions()
	if err != nil {
		return "", err
	}

	var matches []string
	for _, s := range sessions {
		if strings.HasPrefix(s.ID, input) {
			matches = append(matches, s.ID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session found matching '%s'", input)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous session ID '%s' — matches %d sessions", input, len(matches))
	}
}

func printSessionSummary(s *api.Session) {
	fmt.Printf("  ID:        %s\n", s.ID)
	fmt.Printf("  Status:    %s\n", output.StatusColor(s.Status))
	if s.Label != "" {
		fmt.Printf("  Label:     %s\n", s.Label)
	}
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

	// Show tunnel connection details
	tunnels := getTunnelResources(s.ResourceIps)
	if len(tunnels) > 0 {
		fmt.Println()
		fmt.Println("  Tunnel resources:")
		for _, t := range tunnels {
			fmt.Printf("    %-20s %s\n", t.name, t.connection)
		}
	}
}

func printSessionDetail(s *api.Session) {
	fmt.Printf("Session %s\n", s.ID)
	fmt.Printf("  Status:    %s\n", output.StatusColor(s.Status))
	fmt.Printf("  User:      %s (%s)\n", s.UserName, s.UserEmail)
	if s.Label != "" {
		fmt.Printf("  Label:     %s\n", s.Label)
	}
	if s.Scoped {
		fmt.Printf("  Scoped:    yes\n")
	}
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
			name := r.ResourceName
			if isTunnelResource(r) {
				port := parseTunnelPort(r.ProviderRuleId)
				if port != "" {
					name += "  edge.entryguard.io:" + port
				}
			}
			rows = append(rows, []string{
				name,
				fmt.Sprintf("IPv%d", r.IpVersion),
				r.IpAddress,
				output.StatusColor(r.Status),
				output.FormatTime(r.AppliedAt),
			})
		}
		output.PrintTable([]string{"RESOURCE", "VERSION", "IP", "STATUS", "APPLIED"}, rows)
	}
}

type tunnelInfo struct {
	name       string
	connection string
}

func getTunnelResources(resourceIps []api.SessionResourceIp) []tunnelInfo {
	seen := make(map[string]bool)
	var tunnels []tunnelInfo
	for _, r := range resourceIps {
		if !isTunnelResource(r) || seen[r.ResourceName] {
			continue
		}
		seen[r.ResourceName] = true
		port := parseTunnelPort(r.ProviderRuleId)
		if port != "" {
			tunnels = append(tunnels, tunnelInfo{
				name:       r.ResourceName,
				connection: "edge.entryguard.io:" + port,
			})
		}
	}
	return tunnels
}

func isTunnelResource(r api.SessionResourceIp) bool {
	return strings.HasPrefix(r.ProviderRuleId, "tunnel:")
}

func parseTunnelPort(providerRuleId string) string {
	// Format: "tunnel:<port>:<cidr>"
	parts := strings.SplitN(providerRuleId, ":", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func init() {
	sessionStartCmd.Flags().StringVar(&sessionDuration, "duration", "", "Session length: hours (4, 4h) or minutes (15m, 1h30m); default: your org's default")
	sessionStartCmd.Flags().StringSliceVar(&sessionResources, "resources", nil, "Only open these resources (names or ids, comma-separated or repeated)")
	sessionStartCmd.Flags().StringVar(&sessionLabel, "label", "", "Label shown in Session activity and the audit log (e.g. the CI run URL)")
	sessionStartCmd.Flags().BoolVar(&sessionWait, "wait", false, "Wait until the IP rules are applied (status leaves PENDING) before returning")
	sessionStartCmd.Flags().DurationVar(&sessionWaitFor, "wait-timeout", 60*time.Second, "How long --wait waits before giving up")
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
