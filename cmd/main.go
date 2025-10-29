package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/d0ugal/github-exporter/internal/collectors"
	"github.com/d0ugal/github-exporter/internal/config"
	"github.com/d0ugal/github-exporter/internal/metrics"
	"github.com/d0ugal/github-exporter/internal/version"
	"github.com/d0ugal/promexporter/app"
	"github.com/d0ugal/promexporter/logging"
	promexporter_metrics "github.com/d0ugal/promexporter/metrics"
)

// hasEnvironmentVariables checks if any GITHUB_EXPORTER_* environment variables are set
func hasEnvironmentVariables() bool {
	envVars := []string{
		"GITHUB_EXPORTER_SERVER_HOST",
		"GITHUB_EXPORTER_SERVER_PORT",
		"GITHUB_EXPORTER_LOG_LEVEL",
		"GITHUB_EXPORTER_LOG_FORMAT",
		"GITHUB_EXPORTER_METRICS_DEFAULT_INTERVAL",
		"GITHUB_EXPORTER_GITHUB_TOKEN",
		"GITHUB_EXPORTER_GITHUB_ORGS",
		"GITHUB_EXPORTER_GITHUB_REPOS",
		"GITHUB_EXPORTER_GITHUB_TIMEOUT",
		"GITHUB_EXPORTER_GITHUB_RATE_LIMIT",
	}

	for _, envVar := range envVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}

	return false
}

func main() {
	// Parse command line flags
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.BoolVar(&showVersion, "v", false, "Show version information")

	var (
		configPath    string
		configFromEnv bool
	)

	flag.StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	flag.BoolVar(&configFromEnv, "config-from-env", false, "Load configuration from environment variables only")
	flag.Parse()

	// Show version if requested
	if showVersion {
		fmt.Printf("github-exporter %s\n", version.Version)
		fmt.Printf("Commit: %s\n", version.Commit)
		fmt.Printf("Build Date: %s\n", version.BuildDate)
		os.Exit(0)
	}

	// Use environment variable if config flag is not provided
	if configPath == "config.yaml" && !configFromEnv {
		if envConfig := os.Getenv("CONFIG_PATH"); envConfig != "" {
			configPath = envConfig
		}
	}

	// Check if we should use environment-only configuration
	if !configFromEnv {
		// Check explicit flag first
		if os.Getenv("GITHUB_EXPORTER_CONFIG_FROM_ENV") == "true" {
			configFromEnv = true
		} else if hasEnvironmentVariables() {
			// Auto-detect environment variables and use them
			configFromEnv = true
		}
	}

	// Load configuration
	cfg, err := config.LoadConfig(configPath, configFromEnv)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Configure logging using promexporter
	logging.Configure(&logging.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	})

	// Initialize metrics registry using promexporter
	metricsRegistry := promexporter_metrics.NewRegistry("github_exporter_info")

	// Add custom metrics to the registry
	githubRegistry := metrics.NewGitHubRegistry(metricsRegistry)

	// Create and build application using promexporter
	application := app.New("github-exporter").
		WithConfig(&cfg.BaseConfig).
		WithMetrics(metricsRegistry).
		WithVersionInfo(version.Version, version.Commit, version.BuildDate).
		Build()

	// Create collector with app reference for tracing
	githubCollector := collectors.NewGitHubCollector(cfg, githubRegistry, application)
	application.WithCollector(githubCollector)

	if err := application.Run(); err != nil {
		slog.Error("Application failed", "error", err)
		os.Exit(1)
	}
}
