package collectors

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/d0ugal/github-exporter/internal/config"
	"github.com/d0ugal/github-exporter/internal/metrics"
	"github.com/google/go-github/v76/github"
	"golang.org/x/time/rate"
)

type GitHubCollector struct {
	config  *config.Config
	metrics *metrics.GitHubRegistry
	client  *github.Client
	limiter *rate.Limiter
	mu      sync.RWMutex

	// Rate limiting state
	rateLimitTotal     int
	rateLimitRemaining int
	rateLimitReset     time.Time
	lastRateLimitCheck time.Time
}

func NewGitHubCollector(cfg *config.Config, metricsRegistry *metrics.GitHubRegistry) *GitHubCollector {
	// Create GitHub client
	client := github.NewClient(nil).WithAuthToken(cfg.GitHub.Token)

	// Create initial conservative rate limiter - will be updated dynamically based on actual API limits
	// Start with a very conservative rate (1 request per second)
	limiter := rate.NewLimiter(1, 1)

	return &GitHubCollector{
		config:  cfg,
		metrics: metricsRegistry,
		client:  client,
		limiter: limiter,
	}
}

func (gc *GitHubCollector) Start(ctx context.Context) {
	go gc.run(ctx)
}

func (gc *GitHubCollector) run(ctx context.Context) {
	// Run immediately on start
	gc.collectMetrics(ctx)

	// Calculate initial refresh interval
	refreshInterval := gc.calculateRefreshInterval()

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Shutting down GitHub collector")
			return
		case <-ticker.C:
			gc.collectMetrics(ctx)

			// Recalculate refresh interval based on current rate limits
			newInterval := gc.calculateRefreshInterval()
			if newInterval != refreshInterval {
				slog.Info("Updating refresh interval", "old", refreshInterval, "new", newInterval)
				refreshInterval = newInterval
				ticker.Reset(refreshInterval)
			}
		}
	}
}

func (gc *GitHubCollector) collectMetrics(ctx context.Context) {
	slog.Debug("Collecting GitHub metrics")

	// Check and update rate limits first
	if err := gc.updateRateLimits(ctx); err != nil {
		slog.Error("Failed to update rate limits", "error", err)
		gc.metrics.GitHubAPIErrorsTotal.WithLabelValues("rate_limit", "update_error").Inc()
		return
	}

	// Wait for rate limiter
	if err := gc.limiter.Wait(ctx); err != nil {
		slog.Error("Rate limiter error", "error", err)
		return
	}

	// Collect organization metrics
	if err := gc.collectOrgMetrics(ctx); err != nil {
		slog.Error("Failed to collect organization metrics", "error", err)
		gc.metrics.GitHubAPIErrorsTotal.WithLabelValues("orgs", "collection_error").Inc()
	}

	// Collect repository metrics
	if err := gc.collectRepoMetrics(ctx); err != nil {
		slog.Error("Failed to collect repository metrics", "error", err)
		gc.metrics.GitHubAPIErrorsTotal.WithLabelValues("repos", "collection_error").Inc()
	}

	slog.Debug("GitHub metrics collection completed")
}

// calculateRefreshInterval calculates the optimal refresh interval based on rate limits
func (gc *GitHubCollector) calculateRefreshInterval() time.Duration {
	// If a specific refresh interval is configured, use it
	if gc.config.GitHub.RefreshInterval.Duration > 0 {
		return gc.config.GitHub.RefreshInterval.Duration
	}

	gc.mu.RLock()
	defer gc.mu.RUnlock()

	// If we don't have rate limit info yet, use a conservative default
	if gc.rateLimitRemaining == 0 || gc.rateLimitTotal == 0 {
		return time.Duration(gc.config.GetDefaultInterval()) * time.Second
	}

	// Calculate how many API calls we need per collection cycle
	// Each org requires: 1 call for org info + 1 call for repos
	// Each specific repo requires: 1 call
	totalCallsPerCycle := len(gc.config.GitHub.Orgs)*2 + len(gc.config.GitHub.Repos)

	// Add 1 for rate limit check
	totalCallsPerCycle++

	// Calculate how many cycles we can do with remaining rate limit
	// Apply buffer to stay under limit
	availableCalls := int(float64(gc.rateLimitRemaining) * gc.config.GitHub.RateLimitBuffer)

	if availableCalls <= 0 {
		// If no calls available, wait until rate limit resets
		timeUntilReset := time.Until(gc.rateLimitReset)
		if timeUntilReset > 0 {
			return timeUntilReset
		}
		// Fallback to default if reset time is in the past
		return time.Duration(gc.config.GetDefaultInterval()) * time.Second
	}

	cyclesPossible := availableCalls / totalCallsPerCycle
	if cyclesPossible <= 0 {
		cyclesPossible = 1
	}

	// Calculate interval: distribute remaining time evenly across possible cycles
	timeUntilReset := time.Until(gc.rateLimitReset)
	if timeUntilReset <= 0 {
		timeUntilReset = time.Hour // Default to 1 hour if reset time is invalid
	}

	interval := timeUntilReset / time.Duration(cyclesPossible)

	// Ensure minimum interval of 30 seconds
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}

	// Ensure maximum interval of 1 hour
	if interval > time.Hour {
		interval = time.Hour
	}

	slog.Debug("Calculated refresh interval",
		"interval", interval,
		"rate_limit_remaining", gc.rateLimitRemaining,
		"calls_per_cycle", totalCallsPerCycle,
		"cycles_possible", cyclesPossible,
		"time_until_reset", timeUntilReset)

	return interval
}

// updateRateLimits fetches current rate limit information from GitHub
func (gc *GitHubCollector) updateRateLimits(ctx context.Context) error {
	// Only check rate limits if it's been more than 5 minutes since last check
	// or if we've never checked before
	gc.mu.RLock()
	lastCheck := gc.lastRateLimitCheck
	gc.mu.RUnlock()

	if time.Since(lastCheck) < 5*time.Minute && !lastCheck.IsZero() {
		return nil // Skip rate limit check
	}

	// Wait for rate limiter
	if err := gc.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}

	// Get rate limit information using the new API
	rateLimit, resp, err := gc.client.RateLimit.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get rate limit info: %w", err)
	}

	// Update API call metrics
	gc.metrics.GitHubAPICallsTotal.WithLabelValues("rate_limit", fmt.Sprintf("%d", resp.StatusCode)).Inc()

	// Update rate limit state
	gc.mu.Lock()
	if rateLimit.Core != nil {
		gc.rateLimitTotal = rateLimit.Core.Limit
		gc.rateLimitRemaining = rateLimit.Core.Remaining
		if !rateLimit.Core.Reset.IsZero() {
			gc.rateLimitReset = rateLimit.Core.Reset.Time
		}
	}
	gc.lastRateLimitCheck = time.Now()
	gc.mu.Unlock()

	// Update rate limit metrics
	if rateLimit.Core != nil {
		gc.metrics.GitHubRateLimitTotal.WithLabelValues().Set(float64(rateLimit.Core.Limit))
		gc.metrics.GitHubRateLimitRemaining.WithLabelValues().Set(float64(rateLimit.Core.Remaining))
		if !rateLimit.Core.Reset.IsZero() {
			gc.metrics.GitHubRateLimitReset.WithLabelValues().Set(float64(rateLimit.Core.Reset.Unix()))
		}
	}

	// Update rate limiter based on current limits
	gc.updateRateLimiter()

	return nil
}

// updateRateLimiter updates the rate limiter based on current rate limit information
func (gc *GitHubCollector) updateRateLimiter() {
	gc.mu.RLock()
	remaining := gc.rateLimitRemaining
	resetTime := gc.rateLimitReset
	gc.mu.RUnlock()

	if remaining <= 0 || resetTime.IsZero() {
		return
	}

	// Calculate time until reset
	timeUntilReset := time.Until(resetTime)
	if timeUntilReset <= 0 {
		timeUntilReset = time.Hour // Default to 1 hour
	}

	// Calculate rate: remaining requests / time until reset
	// Apply buffer to stay under limit
	effectiveRemaining := int(float64(remaining) * gc.config.GitHub.RateLimitBuffer)
	ratePerSecond := float64(effectiveRemaining) / timeUntilReset.Seconds()

	// Create new rate limiter
	newLimiter := rate.NewLimiter(rate.Limit(ratePerSecond), 1)

	gc.mu.Lock()
	gc.limiter = newLimiter
	gc.mu.Unlock()

	slog.Debug("Updated rate limiter",
		"rate_per_second", ratePerSecond,
		"remaining", remaining,
		"time_until_reset", timeUntilReset,
		"effective_remaining", effectiveRemaining)
}

func (gc *GitHubCollector) collectOrgMetrics(ctx context.Context) error {
	// Collect metrics for each organization
	for _, org := range gc.config.GitHub.Orgs {
		// Wait for rate limiter
		if err := gc.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter error: %w", err)
		}

		// Get organization information
		orgInfo, resp, err := gc.client.Organizations.Get(ctx, org)
		if err != nil {
			slog.Error("Failed to get organization info", "org", org, "error", err)
			gc.metrics.GitHubAPIErrorsTotal.WithLabelValues("orgs", "api_error").Inc()
			continue
		}

		// Update API call metrics
		gc.metrics.GitHubAPICallsTotal.WithLabelValues("orgs", fmt.Sprintf("%d", resp.StatusCode)).Inc()

		// Set organization metrics
		if orgInfo.PublicRepos != nil {
			gc.metrics.GitHubOrgsPublicRepos.WithLabelValues(org).Set(float64(*orgInfo.PublicRepos))
		}
		if orgInfo.Followers != nil {
			gc.metrics.GitHubOrgsFollowers.WithLabelValues(org).Set(float64(*orgInfo.Followers))
		}
		if orgInfo.Following != nil {
			gc.metrics.GitHubOrgsFollowing.WithLabelValues(org).Set(float64(*orgInfo.Following))
		}

		// Get repositories for the organization
		if err := gc.collectOrgRepos(ctx, org); err != nil {
			slog.Error("Failed to collect organization repositories", "org", org, "error", err)
		}
	}

	// Set total organizations count
	gc.metrics.GitHubOrgsTotal.WithLabelValues().Set(float64(len(gc.config.GitHub.Orgs)))

	return nil
}

func (gc *GitHubCollector) collectOrgRepos(ctx context.Context, org string) error {
	// Wait for rate limiter
	if err := gc.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}

	// List repositories for the organization
	repos, resp, err := gc.client.Repositories.ListByOrg(ctx, org, &github.RepositoryListByOrgOptions{
		Type: "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list repositories for org %s: %w", org, err)
	}

	// Update API call metrics
	gc.metrics.GitHubAPICallsTotal.WithLabelValues("repos", fmt.Sprintf("%d", resp.StatusCode)).Inc()

	// Count repositories by visibility
	publicCount := 0
	privateCount := 0

	for _, repo := range repos {
		visibility := "public"
		if repo.Private != nil && *repo.Private {
			visibility = "private"
		}

		if visibility == "public" {
			publicCount++
		} else {
			privateCount++
		}

		// Set repository metrics
		gc.setRepoMetrics(ctx, org, *repo.Name, visibility, repo)
	}

	// Set repository counts
	gc.metrics.GitHubReposTotal.WithLabelValues(org, "public").Set(float64(publicCount))
	gc.metrics.GitHubReposTotal.WithLabelValues(org, "private").Set(float64(privateCount))

	return nil
}

func (gc *GitHubCollector) collectRepoMetrics(ctx context.Context) error {
	// Collect metrics for specific repositories
	for _, repoFullName := range gc.config.GitHub.Repos {
		parts := strings.Split(repoFullName, "/")
		if len(parts) != 2 {
			slog.Error("Invalid repository format", "repo", repoFullName)
			continue
		}

		owner := parts[0]
		repo := parts[1]

		// Wait for rate limiter
		if err := gc.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter error: %w", err)
		}

		// Get repository information
		repoInfo, resp, err := gc.client.Repositories.Get(ctx, owner, repo)
		if err != nil {
			slog.Error("Failed to get repository info", "owner", owner, "repo", repo, "error", err)
			gc.metrics.GitHubAPIErrorsTotal.WithLabelValues("repos", "api_error").Inc()
			continue
		}

		// Update API call metrics
		gc.metrics.GitHubAPICallsTotal.WithLabelValues("repos", fmt.Sprintf("%d", resp.StatusCode)).Inc()

		// Set repository metrics
		visibility := "public"
		if repoInfo.Private != nil && *repoInfo.Private {
			visibility = "private"
		}

		gc.setRepoMetrics(ctx, owner, repo, visibility, repoInfo)
	}

	return nil
}

func (gc *GitHubCollector) setRepoMetrics(ctx context.Context, owner, repo, visibility string, repoInfo *github.Repository) {
	// Repository info metric with labels
	archived := "false"
	if repoInfo.Archived != nil && *repoInfo.Archived {
		archived = "true"
	}
	
	fork := "false"
	if repoInfo.Fork != nil && *repoInfo.Fork {
		fork = "true"
	}
	
	language := ""
	if repoInfo.Language != nil {
		language = *repoInfo.Language
	}
	
	// Set info metric (always 1 for info metrics)
	gc.metrics.GitHubReposInfo.WithLabelValues(owner, repo, visibility, archived, fork, language).Set(1)

	// Stars
	if repoInfo.StargazersCount != nil {
		gc.metrics.GitHubReposStars.WithLabelValues(owner, repo, visibility).Set(float64(*repoInfo.StargazersCount))
	}

	// Forks
	if repoInfo.ForksCount != nil {
		gc.metrics.GitHubReposForks.WithLabelValues(owner, repo, visibility).Set(float64(*repoInfo.ForksCount))
	}

	// Watchers
	if repoInfo.WatchersCount != nil {
		gc.metrics.GitHubReposWatchers.WithLabelValues(owner, repo, visibility).Set(float64(*repoInfo.WatchersCount))
	}

	// Open issues
	if repoInfo.OpenIssuesCount != nil {
		gc.metrics.GitHubReposOpenIssues.WithLabelValues(owner, repo, visibility).Set(float64(*repoInfo.OpenIssuesCount))
	}

	// Open PRs - we need to fetch this separately as it's not in the basic repo info
	gc.setOpenPRsMetric(ctx, owner, repo, visibility)

	// Size
	if repoInfo.Size != nil {
		gc.metrics.GitHubReposSize.WithLabelValues(owner, repo, visibility).Set(float64(*repoInfo.Size))
	}

	// Last updated
	if repoInfo.UpdatedAt != nil {
		gc.metrics.GitHubReposLastUpdated.WithLabelValues(owner, repo, visibility).Set(float64(repoInfo.UpdatedAt.Unix()))
	}

	// Created at
	if repoInfo.CreatedAt != nil {
		gc.metrics.GitHubReposCreatedAt.WithLabelValues(owner, repo, visibility).Set(float64(repoInfo.CreatedAt.Unix()))
	}
}

// setOpenPRsMetric fetches and sets the open PRs count for a repository
func (gc *GitHubCollector) setOpenPRsMetric(ctx context.Context, owner, repo, visibility string) {
	// Wait for rate limiter
	if err := gc.limiter.Wait(ctx); err != nil {
		slog.Error("Rate limiter error while fetching PRs", "owner", owner, "repo", repo, "error", err)
		return
	}

	// Get open pull requests count
	prs, resp, err := gc.client.PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{
		State: "open",
		ListOptions: github.ListOptions{
			PerPage: 1, // We only need the count, not the actual PRs
		},
	})
	if err != nil {
		slog.Error("Failed to get open PRs", "owner", owner, "repo", repo, "error", err)
		gc.metrics.GitHubAPIErrorsTotal.WithLabelValues("pull_requests", "api_error").Inc()
		return
	}

	// Update API call metrics
	if resp != nil {
		gc.metrics.GitHubAPICallsTotal.WithLabelValues("pull_requests", fmt.Sprintf("%d", resp.StatusCode)).Inc()
	}

	// Set the open PRs count
	// Note: GitHub API doesn't provide a direct count, so we use the total count from the response
	// For a more accurate count, we'd need to paginate through all results, but that would use more API calls
	openPRsCount := 0
	if resp != nil && resp.LastPage > 0 {
		// Estimate based on pagination info
		openPRsCount = resp.LastPage * 30 // GitHub default per_page is 30
	} else if len(prs) > 0 {
		// If we got results, we know there are at least some PRs
		openPRsCount = len(prs)
	}
	
	gc.metrics.GitHubReposOpenPRs.WithLabelValues(owner, repo, visibility).Set(float64(openPRsCount))
}

// Stop stops the collector
func (gc *GitHubCollector) Stop() {
	// No cleanup needed for GitHub collector
}
