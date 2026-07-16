package collectors

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/d0ugal/github-exporter/internal/config"
	"github.com/d0ugal/github-exporter/internal/metrics"
	"github.com/d0ugal/promexporter/app"
	promexporter_config "github.com/d0ugal/promexporter/config"
	promexporter_metrics "github.com/d0ugal/promexporter/metrics"
	"github.com/google/go-github/v89/github"
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

	collector, err := NewGitHubCollector(cfg, metricsRegistry, nil)
	if err != nil {
		t.Fatalf("NewGitHubCollector: %v", err)
	}

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

// TestNewGitHubCollector_WithAuthToken locks in that go-github v88's
// WithAuthToken option flows through cleanly when a real token is set.
// v88 rejects an empty WithAuthToken at construction; we side-step that
// in NewGitHubCollector by only attaching the option when a token is
// present, so both paths (token / no-token) need to be exercised.
func TestNewGitHubCollector_WithAuthToken(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: promexporter_config.NewSensitiveString("ghp_dummy_token_for_test"),
			Orgs:  []string{"test-org"},
		},
	}

	baseRegistry := promexporter_metrics.NewRegistry("github-exporter-test")
	metricsRegistry := metrics.NewGitHubRegistry(baseRegistry)

	collector, err := NewGitHubCollector(cfg, metricsRegistry, nil)
	if err != nil {
		t.Fatalf("NewGitHubCollector with token: %v", err)
	}

	if collector == nil || collector.client == nil {
		t.Fatal("collector and underlying client should be initialised")
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

			collector, err := NewGitHubCollector(cfg, metricsRegistry, nil)
			if err != nil {
				t.Fatalf("NewGitHubCollector: %v", err)
			}

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
	collector, err := NewGitHubCollector(cfg, metricsRegistry, nil)
	if err != nil {
		t.Fatalf("NewGitHubCollector: %v", err)
	}

	if collector.limiter == nil {
		t.Error("Expected rate limiter to be initialized")
	}

	// Test that limiter has reasonable initial values
	// The exact values depend on the implementation, but it should not be nil
}

// TestCollectOrgRepos_PagesAllResults verifies that collectOrgRepos walks
// every Link-rel="next" page from the GitHub API rather than stopping at
// page 1. Without the loop, orgs with more than 100 repositories would
// silently report wrong totals.
func TestCollectOrgRepos_PagesAllResults(t *testing.T) {
	var pageRequests atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/orgs/test-org/repos", func(w http.ResponseWriter, r *http.Request) {
		pageRequests.Add(1)

		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}

		nextLink := func(p int) string {
			return fmt.Sprintf(`<http://%s/orgs/test-org/repos?page=%d>; rel="next"`, r.Host, p)
		}

		switch page {
		case "1":
			w.Header().Set("Link", nextLink(2))
			_, _ = fmt.Fprint(w, `[{"id":1,"name":"repo1","private":false}]`)
		case "2":
			w.Header().Set("Link", nextLink(3))
			_, _ = fmt.Fprint(w, `[{"id":2,"name":"repo2","private":false}]`)
		case "3":
			// final page — no rel="next" Link
			_, _ = fmt.Fprint(w, `[{"id":3,"name":"repo3","private":true}]`)
		default:
			t.Errorf("unexpected page request: %s", page)
			http.Error(w, "unexpected page", http.StatusInternalServerError)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Orgs: []string{"test-org"},
		},
		BaseConfig: promexporter_config.BaseConfig{
			Server:  promexporter_config.ServerConfig{Host: "127.0.0.1", Port: 8080},
			Logging: promexporter_config.LoggingConfig{Level: "info", Format: "json"},
		},
	}
	baseRegistry := promexporter_metrics.NewRegistry("github-exporter-pagination-test")
	metricsRegistry := metrics.NewGitHubRegistry(baseRegistry)
	testApp := app.New("github-exporter-pagination-test").
		WithConfig(&cfg.BaseConfig).
		WithMetrics(baseRegistry).
		Build()

	gc, err := NewGitHubCollector(cfg, metricsRegistry, testApp)
	if err != nil {
		t.Fatalf("NewGitHubCollector: %v", err)
	}

	// go-github v88 made BaseURL a getter; the test server URL has to
	// be supplied at construction time. Swap the client for one pointed
	// at the httptest server.
	baseURL := server.URL + "/"

	testClient, err := github.NewClient(github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatalf("create test client: %v", err)
	}

	gc.client = testClient

	if err := gc.collectOrgRepos(context.Background(), "test-org"); err != nil {
		t.Fatalf("collectOrgRepos: %v", err)
	}

	if got := pageRequests.Load(); got != 3 {
		t.Fatalf("expected 3 page requests (full pagination), got %d — collector likely stopped early", got)
	}
}

// TestGitHubToken_RedactsInString locks in that GitHub.Token's String()
// emits "[REDACTED]" rather than the raw token. Without this, any code path
// that prints the config struct (slog with %v, error wrapping, etc.) would
// leak the PAT into logs.
func TestGitHubToken_RedactsInString(t *testing.T) {
	const secret = "ghp_supersecrettokenvalue1234"

	tok := promexporter_config.NewSensitiveString(secret)

	if tok.Value() != secret {
		t.Fatalf("Value() did not round-trip: want %q, got %q", secret, tok.Value())
	}

	if got := tok.String(); got == secret {
		t.Fatalf("String() leaked the raw token: %q", got)
	}

	if got := tok.String(); got != "[REDACTED]" {
		t.Fatalf("String() unexpected: want [REDACTED], got %q", got)
	}
}

// TestNewGitHubCollector_WiresHTTPTimeout asserts that the configured
// GitHub.Timeout flows through into the underlying *http.Client. Without
// this, github.NewClient(nil) used http.DefaultClient (no timeout) and a
// hung TCP connection would block the collector indefinitely.
func TestNewGitHubCollector_WiresHTTPTimeout(t *testing.T) {
	wantTimeout := 17 * time.Second

	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Timeout: promexporter_config.Duration{Duration: wantTimeout},
		},
	}
	baseRegistry := promexporter_metrics.NewRegistry("github-exporter-timeout-test")
	metricsRegistry := metrics.NewGitHubRegistry(baseRegistry)

	gc, err := NewGitHubCollector(cfg, metricsRegistry, nil)
	if err != nil {
		t.Fatalf("NewGitHubCollector: %v", err)
	}

	if gc.client.Client().Timeout != wantTimeout {
		t.Fatalf("expected http client timeout %v, got %v", wantTimeout, gc.client.Client().Timeout)
	}
}

// TestMetricsRegistry tests that metrics registry is properly set up
// TestUpdateRateLimiter_NoRace exercises the rate-limiter mutation path
// concurrently with the Wait() readers used throughout the collector. With
// the previous pointer-swap implementation, `go test -race` would flag this
// immediately. With the in-place SetLimit/SetBurst fix the rate.Limiter's
// own mutex serialises everything.
func TestUpdateRateLimiter_NoRace(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			RateLimitBuffer: 0.8,
		},
	}

	baseRegistry := promexporter_metrics.NewRegistry("github-exporter-race-test")
	metricsRegistry := metrics.NewGitHubRegistry(baseRegistry)

	gc, err := NewGitHubCollector(cfg, metricsRegistry, nil)
	if err != nil {
		t.Fatalf("NewGitHubCollector: %v", err)
	}

	gc.rateLimitRemaining = 1000
	gc.rateLimitReset = time.Now().Add(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()

			gc.updateRateLimiter()
		}()
		go func() {
			defer wg.Done()

			_ = gc.limiter.Wait(ctx)
		}()
	}

	wg.Wait()
}

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
