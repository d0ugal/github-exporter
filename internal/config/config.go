package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	promexporter_config "github.com/d0ugal/promexporter/config"
	"gopkg.in/yaml.v3"
)

// Duration uses promexporter Duration type
type Duration = promexporter_config.Duration

type Config struct {
	promexporter_config.BaseConfig

	GitHub GitHubConfig `yaml:"github"`
}

type GitHubConfig struct {
	Token           string   `yaml:"token"`
	Orgs            []string `yaml:"orgs"`
	Repos           []string `yaml:"repos"`
	Branches        []string `yaml:"branches"`  // Branches to monitor for build status
	Workflows       []string `yaml:"workflows"` // Specific workflows to monitor (empty = all)
	Timeout         Duration `yaml:"timeout"`
	RefreshInterval Duration `yaml:"refresh_interval"`
	RateLimitBuffer float64  `yaml:"rate_limit_buffer"` // Percentage to stay under limit (0.8 = 80%)
}

// LoadConfig loads configuration with priority: env vars > yaml file > defaults.
// The yaml file is optional; if path is empty or the file does not exist it is
// silently skipped. Environment variables are always applied on top.
func LoadConfig(path string) (*Config, error) {
	var cfg Config

	// Optional yaml — silently skip if not found
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
		}
	}

	// Always apply generic env vars (TRACING_ENABLED, PROFILING_ENABLED, etc.)
	if err := promexporter_config.ApplyGenericEnvVars(&cfg.BaseConfig); err != nil {
		return nil, fmt.Errorf("failed to apply generic environment variables: %w", err)
	}

	// Always apply GitHub-specific env vars on top
	applyEnvVars(&cfg)

	setDefaults(&cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

// applyEnvVars overlays GitHub-exporter environment variables onto cfg.
// Only variables that are set (non-empty) are applied; unset variables leave
// the existing value (from yaml or zero-value) unchanged.
func applyEnvVars(cfg *Config) {
	if host := os.Getenv("GITHUB_EXPORTER_SERVER_HOST"); host != "" {
		cfg.Server.Host = host
	}
	if portStr := os.Getenv("GITHUB_EXPORTER_SERVER_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			cfg.Server.Port = port
		}
	}
	if level := os.Getenv("GITHUB_EXPORTER_LOG_LEVEL"); level != "" {
		cfg.Logging.Level = level
	}
	if format := os.Getenv("GITHUB_EXPORTER_LOG_FORMAT"); format != "" {
		cfg.Logging.Format = format
	}
	if intervalStr := os.Getenv("GITHUB_EXPORTER_METRICS_DEFAULT_INTERVAL"); intervalStr != "" {
		if interval, err := time.ParseDuration(intervalStr); err == nil {
			cfg.Metrics.Collection.DefaultInterval = promexporter_config.Duration{Duration: interval}
			cfg.Metrics.Collection.DefaultIntervalSet = true
		}
	}
	if token := os.Getenv("GITHUB_EXPORTER_GITHUB_TOKEN"); token != "" {
		cfg.GitHub.Token = token
	}
	if orgsStr := os.Getenv("GITHUB_EXPORTER_GITHUB_ORGS"); orgsStr != "" {
		cfg.GitHub.Orgs = strings.Split(orgsStr, ",")
	}
	if reposStr := os.Getenv("GITHUB_EXPORTER_GITHUB_REPOS"); reposStr != "" {
		cfg.GitHub.Repos = strings.Split(reposStr, ",")
	}
	if branchesStr := os.Getenv("GITHUB_EXPORTER_GITHUB_BRANCHES"); branchesStr != "" {
		cfg.GitHub.Branches = strings.Split(branchesStr, ",")
	}
	if workflowsStr := os.Getenv("GITHUB_EXPORTER_GITHUB_WORKFLOWS"); workflowsStr != "" {
		cfg.GitHub.Workflows = strings.Split(workflowsStr, ",")
	}
	if timeoutStr := os.Getenv("GITHUB_EXPORTER_GITHUB_TIMEOUT"); timeoutStr != "" {
		if timeout, err := time.ParseDuration(timeoutStr); err == nil {
			cfg.GitHub.Timeout = Duration{Duration: timeout}
		}
	}
	if refreshIntervalStr := os.Getenv("GITHUB_EXPORTER_GITHUB_REFRESH_INTERVAL"); refreshIntervalStr != "" {
		if refreshInterval, err := time.ParseDuration(refreshIntervalStr); err == nil {
			cfg.GitHub.RefreshInterval = Duration{Duration: refreshInterval}
		}
	}
	if bufferStr := os.Getenv("GITHUB_EXPORTER_GITHUB_RATE_LIMIT_BUFFER"); bufferStr != "" {
		if buffer, err := strconv.ParseFloat(bufferStr, 64); err == nil {
			cfg.GitHub.RateLimitBuffer = buffer
		}
	}
}

// setDefaults sets default values for configuration
func setDefaults(config *Config) {
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}

	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}

	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}

	if config.Logging.Format == "" {
		config.Logging.Format = "json"
	}

	if !config.Metrics.Collection.DefaultIntervalSet {
		config.Metrics.Collection.DefaultInterval = promexporter_config.Duration{Duration: time.Second * 30}
	}

	if config.GitHub.Timeout.Duration == 0 {
		config.GitHub.Timeout = Duration{Duration: time.Second * 30}
	}

	if config.GitHub.RefreshInterval.Duration == 0 {
		// Will be calculated dynamically based on rate limits
		config.GitHub.RefreshInterval = Duration{Duration: 0}
	}

	if config.GitHub.RateLimitBuffer == 0 {
		config.GitHub.RateLimitBuffer = 0.8
	}
}

// Validate performs comprehensive validation of the configuration
func (c *Config) Validate() error {
	// Validate server configuration
	if err := c.validateServerConfig(); err != nil {
		return fmt.Errorf("server config: %w", err)
	}

	// Validate logging configuration
	if err := c.validateLoggingConfig(); err != nil {
		return fmt.Errorf("logging config: %w", err)
	}

	// Validate metrics configuration
	if err := c.validateMetricsConfig(); err != nil {
		return fmt.Errorf("metrics config: %w", err)
	}

	// Validate GitHub configuration
	if err := c.validateGitHubConfig(); err != nil {
		return fmt.Errorf("github config: %w", err)
	}

	return nil
}

func (c *Config) validateServerConfig() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", c.Server.Port)
	}

	return nil
}

func (c *Config) validateLoggingConfig() error {
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid logging level: %s", c.Logging.Level)
	}

	validFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid logging format: %s", c.Logging.Format)
	}

	return nil
}

func (c *Config) validateMetricsConfig() error {
	if c.Metrics.Collection.DefaultInterval.Seconds() < 1 {
		return fmt.Errorf("default interval must be at least 1 second, got %d", c.Metrics.Collection.DefaultInterval.Seconds())
	}

	if c.Metrics.Collection.DefaultInterval.Seconds() > 86400 {
		return fmt.Errorf("default interval must be at most 86400 seconds (24 hours), got %d", c.Metrics.Collection.DefaultInterval.Seconds())
	}

	return nil
}

func (c *Config) validateGitHubConfig() error {
	if c.GitHub.Token == "" {
		return fmt.Errorf("github token is required")
	}

	if len(c.GitHub.Orgs) == 0 && len(c.GitHub.Repos) == 0 {
		return fmt.Errorf("at least one GitHub organization or repository must be specified")
	}

	// Validate branches configuration
	for _, branch := range c.GitHub.Branches {
		if strings.TrimSpace(branch) == "" {
			return fmt.Errorf("branch names cannot be empty")
		}
	}

	// Validate workflows configuration
	for _, workflow := range c.GitHub.Workflows {
		if strings.TrimSpace(workflow) == "" {
			return fmt.Errorf("workflow names cannot be empty")
		}
	}

	if c.GitHub.Timeout.Seconds() < 1 {
		return fmt.Errorf("github timeout must be at least 1 second, got %d", c.GitHub.Timeout.Seconds())
	}

	if c.GitHub.RateLimitBuffer <= 0 || c.GitHub.RateLimitBuffer > 1 {
		return fmt.Errorf("github rate limit buffer must be between 0 and 1, got %f", c.GitHub.RateLimitBuffer)
	}

	return nil
}

// GetDefaultInterval returns the default collection interval
func (c *Config) GetDefaultInterval() int {
	return c.Metrics.Collection.DefaultInterval.Seconds()
}

// ParseStringList parses a comma-separated string into a slice of strings
func ParseStringList(input string) []string {
	if input == "" {
		return []string{}
	}

	parts := strings.Split(input, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// ParseBool parses a string to boolean
func ParseBool(input string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value: %s", input)
	}
}

// ParseInt parses a string to int
func ParseInt(input string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(input))
}
