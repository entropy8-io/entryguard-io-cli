package agent

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const DefaultConfigPath = "/etc/eg-agent/config.yml"

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Agent     AgentConfig     `yaml:"agent"`
	Scripts   ScriptsConfig   `yaml:"scripts"`
	Execution ExecutionConfig `yaml:"execution"`
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

type ScriptsConfig struct {
	Apply  string `yaml:"apply"`
	Revoke string `yaml:"revoke"`
}

type ExecutionConfig struct {
	Timeout time.Duration `yaml:"timeout"`
	Shell   string        `yaml:"shell"`
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
			Shell:   "/bin/bash",
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
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
