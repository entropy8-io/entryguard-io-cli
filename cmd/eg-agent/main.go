package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/entryguard-io/cli/internal/agent"
	"github.com/spf13/cobra"
)

var (
	version    = "dev"
	configPath string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "eg-agent",
		Short: "EntryGuard Agent — executes IP whitelisting scripts on Linux hosts",
		Long:  "A lightweight agent that polls EntryGuard for commands and executes local scripts to apply/revoke IP rules.",
	}

	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", agent.DefaultConfigPath, "Path to config file")

	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(statusCmd())

	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize agent configuration",
		Long:  "Interactive setup: prompts for API key and server URL, tests connection, registers with EntryGuard, and writes config file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit()
		},
	}
}

func runInit() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("EntryGuard Agent Setup")
	fmt.Println("======================")
	fmt.Println()

	// Server URL
	fmt.Print("Server URL [https://app.entryguard.io/api/v1]: ")
	serverURL, _ := reader.ReadString('\n')
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		serverURL = "https://app.entryguard.io/api/v1"
	}

	// API Key
	fmt.Print("API Key (with agent:connect scope): ")
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return fmt.Errorf("API key is required")
	}

	// Agent name
	hostname, _ := os.Hostname()
	fmt.Printf("Agent name [%s]: ", hostname)
	agentName, _ := reader.ReadString('\n')
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		agentName = hostname
	}

	// Apply script path
	fmt.Print("Apply script path [/etc/eg-agent/scripts/apply.sh]: ")
	applyScript, _ := reader.ReadString('\n')
	applyScript = strings.TrimSpace(applyScript)
	if applyScript == "" {
		applyScript = "/etc/eg-agent/scripts/apply.sh"
	}

	// Revoke script path
	fmt.Print("Revoke script path [/etc/eg-agent/scripts/revoke.sh]: ")
	revokeScript, _ := reader.ReadString('\n')
	revokeScript = strings.TrimSpace(revokeScript)
	if revokeScript == "" {
		revokeScript = "/etc/eg-agent/scripts/revoke.sh"
	}

	// Test connection
	fmt.Println()
	fmt.Println("Testing connection...")
	client := agent.NewClient(serverURL, apiKey)

	resp, err := client.Register(agent.RegisterRequest{
		Name:         agentName,
		AgentVersion: version,
		Hostname:     hostname,
		OsInfo:       runtime.GOOS + "/" + runtime.GOARCH,
	})
	if err != nil {
		return fmt.Errorf("failed to register with EntryGuard: %w", err)
	}

	fmt.Printf("Registered as agent: %s (id=%s)\n", resp.Name, resp.ID)

	// Write config
	cfg := &agent.Config{
		Server: agent.ServerConfig{
			URL:    serverURL,
			APIKey: apiKey,
		},
		Agent: agent.AgentConfig{
			Name: agentName,
		},
		Scripts: agent.ScriptsConfig{
			Apply:  applyScript,
			Revoke: revokeScript,
		},
	}

	if err := agent.WriteConfig(configPath, cfg); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("\nConfig written to %s\n", configPath)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. Create your apply/revoke scripts at:\n")
	fmt.Printf("     - %s\n", applyScript)
	fmt.Printf("     - %s\n", revokeScript)
	fmt.Printf("  2. Start the agent: eg-agent run\n")

	return nil
}

func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Start the agent daemon",
		Long:  "Registers with EntryGuard, starts heartbeat, and polls for commands to execute.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent()
		},
	}
}

func runAgent() error {
	cfg, err := agent.LoadConfig(configPath)
	if err != nil {
		return err
	}

	log.Printf("eg-agent %s starting (name=%s)", version, cfg.Agent.Name)

	client := agent.NewClient(cfg.Server.URL, cfg.Server.APIKey)

	// Register on startup
	hostname, _ := os.Hostname()
	resp, err := client.Register(agent.RegisterRequest{
		Name:         cfg.Agent.Name,
		AgentVersion: version,
		Hostname:     hostname,
		OsInfo:       runtime.GOOS + "/" + runtime.GOARCH,
	})
	if err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}
	log.Printf("registered as %s (id=%s, status=%s)", resp.Name, resp.ID, resp.Status)

	// Start heartbeat
	hb := agent.NewHeartbeater(client, cfg.Agent.HeartbeatInterval, version)
	hb.Start()
	defer hb.Stop()

	// Start poller
	executor := agent.NewExecutor(cfg.Execution.Shell, cfg.Execution.Timeout)
	poller := agent.NewPoller(client, executor, cfg.Scripts, cfg.Agent.PollInterval)

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("shutting down...")
		poller.Stop()
	}()

	poller.Run()
	return nil
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show agent status",
		Long:  "Display current configuration and connection status.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	cfg, err := agent.LoadConfig(configPath)
	if err != nil {
		return err
	}

	fmt.Println("EntryGuard Agent Status")
	fmt.Println("=======================")
	fmt.Printf("Version:  %s\n", version)
	fmt.Printf("Config:   %s\n", configPath)
	fmt.Printf("Server:   %s\n", cfg.Server.URL)
	fmt.Printf("Agent:    %s\n", cfg.Agent.Name)
	fmt.Printf("Apply:    %s\n", cfg.Scripts.Apply)
	fmt.Printf("Revoke:   %s\n", cfg.Scripts.Revoke)
	fmt.Println()

	// Test connection
	fmt.Println("Checking connection...")
	client := agent.NewClient(cfg.Server.URL, cfg.Server.APIKey)

	hostname, _ := os.Hostname()
	resp, err := client.Heartbeat(agent.HeartbeatRequest{
		AgentVersion: version,
		Hostname:     hostname,
		OsInfo:       runtime.GOOS + "/" + runtime.GOARCH,
	})
	if err != nil {
		fmt.Printf("Connection: FAILED (%v)\n", err)
		return nil
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Printf("Connection: OK\n")
	fmt.Printf("Agent info: %s\n", string(data))
	return nil
}
