package agent

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	DefaultConfigPath string
	DefaultShell      string
)

func init() {
	if runtime.GOOS == "windows" {
		DefaultConfigPath = `C:\eg-agent\config.yml`
		DefaultShell = "powershell.exe"
	} else {
		DefaultConfigPath = "/etc/eg-agent/config.yml"
		DefaultShell = "/bin/bash"
	}
}

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Agent     AgentConfig     `yaml:"agent"`
	Execution ExecutionConfig `yaml:"execution"`
	Tunnel    TunnelConfig    `yaml:"tunnel"`
}

type ServerConfig struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

type AgentConfig struct {
	Name              string        `yaml:"name"`
	PollInterval      time.Duration `yaml:"poll_interval"`
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
}

type ExecutionConfig struct {
	Timeout time.Duration `yaml:"timeout"`
	Shell   string        `yaml:"shell"`
}

type TunnelConfig struct {
	Enabled bool   `yaml:"enabled"`
	EdgeURL string `yaml:"edge_url"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	cfg := &Config{
		Agent: AgentConfig{
			PollInterval:      3 * time.Second,
			HeartbeatInterval: 30 * time.Second,
		},
		Execution: ExecutionConfig{
			Timeout: 30 * time.Second,
			Shell:   DefaultShell,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults for zero values (YAML unmarshalling overrides struct defaults)
	if cfg.Agent.PollInterval <= 0 {
		cfg.Agent.PollInterval = 3 * time.Second
	}
	if cfg.Agent.HeartbeatInterval <= 0 {
		cfg.Agent.HeartbeatInterval = 30 * time.Second
	}
	if cfg.Execution.Timeout <= 0 {
		cfg.Execution.Timeout = 30 * time.Second
	}
	if cfg.Execution.Shell == "" {
		cfg.Execution.Shell = DefaultShell
	}

	if cfg.Server.URL == "" {
		return nil, fmt.Errorf("server.url is required")
	}
	if cfg.Server.APIKey == "" {
		return nil, fmt.Errorf("server.api_key is required")
	}
	if cfg.Agent.Name == "" {
		return nil, fmt.Errorf("agent.name is required")
	}

	return cfg, nil
}

func WriteConfig(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	dir := path[:len(path)-len("/config.yml")]
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
