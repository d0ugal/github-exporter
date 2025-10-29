package collectors

import (
	"testing"

	"github.com/d0ugal/github-exporter/internal/config"
	"github.com/d0ugal/github-exporter/internal/metrics"
	promexporter_metrics "github.com/d0ugal/promexporter/metrics"
)

// createTestCollector creates a test GitHubCollector for testing
func createTestCollector() *GitHubCollector {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Repos: []string{},
		},
	}

	baseRegistry := promexporter_metrics.NewRegistry("github-exporter-test")
	metricsRegistry := metrics.NewGitHubRegistry(baseRegistry)

	return &GitHubCollector{
		config:  cfg,
		metrics: metricsRegistry,
	}
}

// TestHasWildcardRepos tests the wildcard detection function
func TestHasWildcardRepos(t *testing.T) {
	collector := createTestCollector()

	// Test with wildcard
	collector.config.GitHub.Repos = []string{"*"}
	if !collector.hasWildcardRepos() {
		t.Error("Expected wildcard repos to be detected")
	}

	// Test without wildcard
	collector.config.GitHub.Repos = []string{"d0ugal/test-repo"}
	if collector.hasWildcardRepos() {
		t.Error("Expected no wildcard repos to be detected")
	}

	// Test with multiple repos including wildcard
	collector.config.GitHub.Repos = []string{"d0ugal/test-repo", "*"}
	if !collector.hasWildcardRepos() {
		t.Error("Expected wildcard repos to be detected")
	}

	// Test with empty repos
	collector.config.GitHub.Repos = []string{}
	if collector.hasWildcardRepos() {
		t.Error("Expected no wildcard repos with empty list")
	}
}

// TestCollectorInitialization tests that the collector initializes properly
func TestCollectorInitialization(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Orgs:     []string{"test-org"},
			Repos:    []string{"test-org/test-repo"},
			Branches: []string{"main"},
		},
	}

	baseRegistry := promexporter_metrics.NewRegistry("github-exporter-test")
	metricsRegistry := metrics.NewGitHubRegistry(baseRegistry)

	collector := NewGitHubCollector(cfg, metricsRegistry, nil)

	if collector == nil {
		t.Fatal("Expected collector to be initialized")
	}
	
	if collector.config == nil {
		t.Error("Expected config to be set")
	}
	
	if collector.metrics == nil {
		t.Error("Expected metrics to be set")
	}
	
	if collector.limiter == nil {
		t.Error("Expected rate limiter to be initialized")
	}
}

// TestConfigValidation tests configuration validation
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name     string
		config   config.GitHubConfig
		expected bool
	}{
		{
			name: "valid config with orgs",
			config: config.GitHubConfig{
				Orgs:  []string{"test-org"},
				Repos: []string{},
			},
			expected: true,
		},
		{
			name: "valid config with repos",
			config: config.GitHubConfig{
				Orgs:  []string{},
				Repos: []string{"test-org/test-repo"},
			},
			expected: true,
		},
		{
			name: "valid config with wildcard",
			config: config.GitHubConfig{
				Orgs:  []string{},
				Repos: []string{"*"},
			},
			expected: true,
		},
		{
			name: "empty config",
			config: config.GitHubConfig{
				Orgs:  []string{},
				Repos: []string{},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				GitHub: tt.config,
			}

			baseRegistry := promexporter_metrics.NewRegistry("github-exporter-test")
			metricsRegistry := metrics.NewGitHubRegistry(baseRegistry)

			collector := NewGitHubCollector(cfg, metricsRegistry, nil)

			if collector == nil {
				t.Error("Expected collector to be initialized")
			}
		})
	}
}

// TestRateLimiterInitialization tests that rate limiter is properly initialized
func TestRateLimiterInitialization(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Repos: []string{},
		},
	}

	baseRegistry := promexporter_metrics.NewRegistry("github-exporter-test")
	metricsRegistry := metrics.NewGitHubRegistry(baseRegistry)

	// Use NewGitHubCollector to ensure rate limiter is initialized
	collector := NewGitHubCollector(cfg, metricsRegistry, nil)

	if collector.limiter == nil {
		t.Error("Expected rate limiter to be initialized")
	}

	// Test that limiter has reasonable initial values
	// The exact values depend on the implementation, but it should not be nil
}

// TestMetricsRegistry tests that metrics registry is properly set up
func TestMetricsRegistry(t *testing.T) {
	collector := createTestCollector()

	if collector.metrics == nil {
		t.Error("Expected metrics registry to be initialized")
	}

	// Test that key metrics are available
	if collector.metrics.GitHubReposInfo == nil {
		t.Error("Expected GitHubReposInfo metric to be available")
	}

	if collector.metrics.GitHubReposStars == nil {
		t.Error("Expected GitHubReposStars metric to be available")
	}

	if collector.metrics.GitHubAPICallsTotal == nil {
		t.Error("Expected GitHubAPICallsTotal metric to be available")
	}
}
