package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/entryguard-io/cli/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run the MCP server for AI assistants (stdio)",
	Long: `Serves EntryGuard sessions to AI assistants over the Model Context Protocol
on stdin/stdout. Tools: whoami, detect_ip, list_resources, list_sessions,
get_session, start_session, stop_session, extend_session.

Authenticate with ENTRYGUARD_API_KEY (and ENTRYGUARD_API_URL for self-hosted),
or with a profile via --profile.`,
	Example: `  # Claude Code
  claude mcp add entryguard --env ENTRYGUARD_API_KEY=eg_live_... -- eg mcp

  # Any MCP client config
  { "mcpServers": { "entryguard": { "command": "eg", "args": ["mcp"], "env": { "ENTRYGUARD_API_KEY": "eg_live_..." } } } }`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := getClient()
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		srv := mcpserver.New(mcpserver.Options{Client: client, Version: rootCmd.Version})
		return srv.Run(ctx, &mcp.StdioTransport{})
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
