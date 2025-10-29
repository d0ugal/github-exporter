package collectors

import (
	"testing"

	"github.com/d0ugal/github-exporter/internal/config"
	"github.com/d0ugal/github-exporter/internal/metrics"
	promexporter_metrics "github.com/d0ugal/promexporter/metrics"
)

// TestHasWildcardRepos tests the wildcard detection function
func TestHasWildcardRepos(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Repos: []string{},
		},
	}
	
	baseRegistry := promexporter_metrics.NewRegistry("github-exporter-test")
	metricsRegistry := metrics.NewGitHubRegistry(baseRegistry)
	
	collector := &GitHubCollector{
		config:  cfg,
		metrics: metricsRegistry,
	}
	
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

