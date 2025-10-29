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
	"github.com/d0ugal/promexporter/app"
	"github.com/d0ugal/promexporter/tracing"
	"github.com/google/go-github/v76/github"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/time/rate"
)

type GitHubCollector struct {
	config  *config.Config
	metrics *metrics.GitHubRegistry
	app     *app.App
	client  *github.Client
	limiter *rate.Limiter
	mu      sync.RWMutex

	// Rate limiting state
	rateLimitTotal     int
	rateLimitRemaining int
	rateLimitReset     time.Time
	lastRateLimitCheck time.Time
}

func NewGitHubCollector(cfg *config.Config, metricsRegistry *metrics.GitHubRegistry, app *app.App) *GitHubCollector {
	// Create GitHub client
	client := github.NewClient(nil).WithAuthToken(cfg.GitHub.Token)

	// Create initial conservative rate limiter - will be updated dynamically based on actual API limits
	// Start with a very conservative rate (1 request per second)
	limiter := rate.NewLimiter(1, 1)

	return &GitHubCollector{
		config:  cfg,
		metrics: metricsRegistry,
		app:     app,
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
	startTime := time.Now()

	slog.Debug("Collecting GitHub metrics")

	// Create span for collection cycle
	tracer := gc.app.GetTracer()

	var collectorSpan *tracing.CollectorSpan

	if tracer != nil && tracer.IsEnabled() {
		collectorSpan = tracer.NewCollectorSpan(ctx, "github-collector", "collect-metrics")
		collectorSpan.SetAttributes(
			attribute.Int("github.orgs_count", len(gc.config.GitHub.Orgs)),
			attribute.Int("github.repos_count", len(gc.config.GitHub.Repos)),
		)
		defer collectorSpan.End()
	}

	// Check and update rate limits first
	if err := gc.updateRateLimits(ctx); err != nil {
		slog.Error("Failed to update rate limits", "error", err)

		if collectorSpan != nil {
			collectorSpan.RecordError(err)
		}

		gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
			"endpoint":   "rate_limit",
			"error_type": "update_error",
		}).Inc()

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
		gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
			"endpoint":   "orgs",
			"error_type": "collection_error",
		}).Inc()
	}

	// Collect repository metrics
	if err := gc.collectRepoMetrics(ctx); err != nil {
		slog.Error("Failed to collect repository metrics", "error", err)
		gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
			"endpoint":   "repos",
			"error_type": "collection_error",
		}).Inc()
	}

	// Collect build status metrics if branches are configured
	if len(gc.config.GitHub.Branches) > 0 {
		if err := gc.collectBuildStatusMetrics(ctx); err != nil {
			slog.Error("Failed to collect build status metrics", "error", err)
			gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
				"endpoint":   "build_status",
				"error_type": "collection_error",
			}).Inc()
		}
	}

	duration := time.Since(startTime).Seconds()

	if collectorSpan != nil {
		collectorSpan.SetAttributes(
			attribute.Float64("collection.duration_seconds", duration),
		)
		collectorSpan.AddEvent("collection_completed",
			attribute.Float64("duration_seconds", duration),
		)
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

	// Add calls for build status metrics if branches are configured
	if len(gc.config.GitHub.Branches) > 0 {
		// Each repo + branch combination requires: 1 call for workflow runs + 1 call for check runs
		totalCallsPerCycle += len(gc.config.GitHub.Repos) * len(gc.config.GitHub.Branches) * 2
	}

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
	gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
		"endpoint": "rate_limit",
		"status":   fmt.Sprintf("%d", resp.StatusCode),
	}).Inc()

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
		gc.metrics.GitHubRateLimitTotal.With(prometheus.Labels{}).Set(float64(rateLimit.Core.Limit))
		gc.metrics.GitHubRateLimitRemaining.With(prometheus.Labels{}).Set(float64(rateLimit.Core.Remaining))
		if !rateLimit.Core.Reset.IsZero() {
			gc.metrics.GitHubRateLimitReset.With(prometheus.Labels{}).Set(float64(rateLimit.Core.Reset.Unix()))
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
			gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
				"endpoint":   "orgs",
				"error_type": "api_error",
			}).Inc()
			// Skip this org entirely - don't collect repos for a non-existent org
			continue
		}

		// Update API call metrics
		gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
			"endpoint": "orgs",
			"status":   fmt.Sprintf("%d", resp.StatusCode),
		}).Inc()

		// Validate organization info before proceeding
		if orgInfo == nil {
			slog.Error("Organization info is nil", "org", org)
			continue
		}

		// Set organization metrics
		if orgInfo.PublicRepos != nil {
			gc.metrics.GitHubOrgsPublicRepos.With(prometheus.Labels{
				"org": org,
			}).Set(float64(*orgInfo.PublicRepos))
		}
		if orgInfo.Followers != nil {
			gc.metrics.GitHubOrgsFollowers.With(prometheus.Labels{
				"org": org,
			}).Set(float64(*orgInfo.Followers))
		}
		if orgInfo.Following != nil {
			gc.metrics.GitHubOrgsFollowing.With(prometheus.Labels{
				"org": org,
			}).Set(float64(*orgInfo.Following))
		}

		// Get repositories for the organization
		// Only collect repos if org fetch was successful
		if err := gc.collectOrgRepos(ctx, org); err != nil {
			slog.Error("Failed to collect organization repositories", "org", org, "error", err)
			// Continue to next org instead of failing completely
			continue
		}
	}

	// Set total organizations count
	gc.metrics.GitHubOrgsTotal.With(prometheus.Labels{}).Set(float64(len(gc.config.GitHub.Orgs)))

	return nil
}

func (gc *GitHubCollector) collectOrgRepos(ctx context.Context, org string) error {
	// Validate org parameter
	if org == "" {
		return fmt.Errorf("org parameter cannot be empty")
	}

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

	// Skip if organization not found (404)
	if resp != nil && resp.StatusCode == 404 {
		slog.Warn("Organization not found, skipping repository collection", "org", org)
		return nil
	}

	// Update API call metrics
	statusCode := "unknown"
	if resp != nil {
		statusCode = fmt.Sprintf("%d", resp.StatusCode)
	}
	gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
		"endpoint": "repos",
		"status":   statusCode,
	}).Inc()

	// Count repositories by visibility
	publicCount := 0
	privateCount := 0

	for _, repo := range repos {
		// Skip repos with missing required fields
		if repo == nil || repo.Name == nil || *repo.Name == "" {
			slog.Warn("Skipping repository with missing or empty name", "org", org)
			continue
		}

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
	gc.metrics.GitHubReposTotal.With(prometheus.Labels{
		"org":        org,
		"visibility": "public",
	}).Set(float64(publicCount))
	gc.metrics.GitHubReposTotal.With(prometheus.Labels{
		"org":        org,
		"visibility": "private",
	}).Set(float64(privateCount))

	return nil
}

func (gc *GitHubCollector) collectRepoMetrics(ctx context.Context) error {
	// Check if wildcard is specified for repos
	if gc.hasWildcardRepos() {
		return gc.collectAllRepos(ctx)
	}

	// Collect metrics for specific repositories
	for _, repoFullName := range gc.config.GitHub.Repos {
		parts := strings.Split(repoFullName, "/")
		if len(parts) != 2 {
			slog.Error("Invalid repository format", "repo", repoFullName)
			continue
		}

		owner := parts[0]
		repo := parts[1]

		// Skip if owner or repo is empty
		if owner == "" || repo == "" {
			slog.Error("Invalid repository format: owner or repo is empty", "repo", repoFullName)
			continue
		}

		// Wait for rate limiter
		if err := gc.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter error: %w", err)
		}

		// Get repository information
		repoInfo, resp, err := gc.client.Repositories.Get(ctx, owner, repo)
		if err != nil {
			slog.Error("Failed to get repository info", "owner", owner, "repo", repo, "error", err)
			gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
				"endpoint":   "repos",
				"error_type": "api_error",
			}).Inc()
			continue
		}

		// Update API call metrics
		gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
			"endpoint": "repos",
			"status":   fmt.Sprintf("%d", resp.StatusCode),
		}).Inc()

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
	// Validate required parameters to prevent panic from missing labels
	if owner == "" {
		slog.Warn("Skipping setRepoMetrics: owner is empty", "repo", repo)
		return
	}
	if repo == "" {
		slog.Warn("Skipping setRepoMetrics: repo is empty", "owner", owner)
		return
	}
	if visibility == "" {
		visibility = "unknown"
	}

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
	gc.metrics.GitHubReposInfo.With(prometheus.Labels{
		"org":        owner,
		"repo":       repo,
		"visibility": visibility,
		"archived":   archived,
		"fork":       fork,
		"language":   language,
	}).Set(1)

	// Stars
	if repoInfo.StargazersCount != nil {
		gc.metrics.GitHubReposStars.With(prometheus.Labels{
			"org":        owner,
			"repo":       repo,
			"visibility": visibility,
		}).Set(float64(*repoInfo.StargazersCount))
	}

	// Forks
	if repoInfo.ForksCount != nil {
		gc.metrics.GitHubReposForks.With(prometheus.Labels{
			"org":        owner,
			"repo":       repo,
			"visibility": visibility,
		}).Set(float64(*repoInfo.ForksCount))
	}

	// Watchers
	if repoInfo.WatchersCount != nil {
		gc.metrics.GitHubReposWatchers.With(prometheus.Labels{
			"org":        owner,
			"repo":       repo,
			"visibility": visibility,
		}).Set(float64(*repoInfo.WatchersCount))
	}

	// Open issues
	if repoInfo.OpenIssuesCount != nil {
		gc.metrics.GitHubReposOpenIssues.With(prometheus.Labels{
			"org":        owner,
			"repo":       repo,
			"visibility": visibility,
		}).Set(float64(*repoInfo.OpenIssuesCount))
	}

	// Open PRs - we need to fetch this separately as it's not in the basic repo info
	gc.setOpenPRsMetric(ctx, owner, repo, visibility)

	// Size
	if repoInfo.Size != nil {
		gc.metrics.GitHubReposSize.With(prometheus.Labels{
			"org":        owner,
			"repo":       repo,
			"visibility": visibility,
		}).Set(float64(*repoInfo.Size))
	}

	// Last updated
	if repoInfo.UpdatedAt != nil {
		gc.metrics.GitHubReposLastUpdated.With(prometheus.Labels{
			"org":        owner,
			"repo":       repo,
			"visibility": visibility,
		}).Set(float64(repoInfo.UpdatedAt.Unix()))
	}

	// Created at
	if repoInfo.CreatedAt != nil {
		gc.metrics.GitHubReposCreatedAt.With(prometheus.Labels{
			"org":        owner,
			"repo":       repo,
			"visibility": visibility,
		}).Set(float64(repoInfo.CreatedAt.Unix()))
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
		gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
			"endpoint":   "pull_requests",
			"error_type": "api_error",
		}).Inc()
		return
	}

	// Update API call metrics
	if resp != nil {
		gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
			"endpoint": "pull_requests",
			"status":   fmt.Sprintf("%d", resp.StatusCode),
		}).Inc()
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

	gc.metrics.GitHubReposOpenPRs.With(prometheus.Labels{
		"org":        owner,
		"repo":       repo,
		"visibility": visibility,
	}).Set(float64(openPRsCount))
}

// hasWildcardRepos checks if "*" is specified in the repos list
func (gc *GitHubCollector) hasWildcardRepos() bool {
	for _, repo := range gc.config.GitHub.Repos {
		if repo == "*" {
			return true
		}
	}
	return false
}

// collectAllRepos collects metrics for all repositories the user has access to
func (gc *GitHubCollector) collectAllRepos(ctx context.Context) error {
	slog.Info("Wildcard repos specified, collecting all accessible repositories")

	// Wait for rate limiter
	if err := gc.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}

	// Get all repositories the authenticated user has access to
	repos, resp, err := gc.client.Repositories.ListByAuthenticatedUser(ctx, &github.RepositoryListByAuthenticatedUserOptions{
		Type: "all",
		ListOptions: github.ListOptions{
			PerPage: 100, // Get more repos per page for efficiency
		},
	})
	if err != nil {
		slog.Error("Failed to list all repositories", "error", err)
		gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
			"endpoint":   "repos",
			"error_type": "api_error",
		}).Inc()
		return fmt.Errorf("failed to list all repositories: %w", err)
	}

	// Update API call metrics
	if resp != nil {
		gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
			"endpoint": "repos",
			"status":   fmt.Sprintf("%d", resp.StatusCode),
		}).Inc()
	}

	// Process each repository
	for _, repo := range repos {
		if repo.Name == nil || repo.Owner == nil || repo.Owner.Login == nil {
			continue
		}

		owner := *repo.Owner.Login
		repoName := *repo.Name

		// Skip if owner or repo name is empty
		if owner == "" || repoName == "" {
			slog.Warn("Skipping repository with empty owner or name", "owner", owner, "repo", repoName)
			continue
		}

		// Determine visibility
		visibility := "public"
		if repo.Private != nil && *repo.Private {
			visibility = "private"
		}

		// Set repository metrics
		gc.setRepoMetrics(ctx, owner, repoName, visibility, repo)
	}

	// Set total repositories count (approximate)
	totalRepos := len(repos)
	if resp != nil && resp.LastPage > 0 {
		// Estimate total based on pagination
		totalRepos = resp.LastPage * 100
	}

	slog.Info("Collected metrics for repositories", "count", len(repos), "estimated_total", totalRepos)

	return nil
}

// collectBuildStatusMetrics collects build status metrics for configured branches
func (gc *GitHubCollector) collectBuildStatusMetrics(ctx context.Context) error {
	// Check if wildcard is specified for repos
	if gc.hasWildcardRepos() {
		return gc.collectBuildStatusForAllRepos(ctx)
	}

	// Collect metrics for specific repositories
	for _, repoFullName := range gc.config.GitHub.Repos {
		parts := strings.Split(repoFullName, "/")
		if len(parts) != 2 {
			slog.Error("Invalid repository format", "repo", repoFullName)
			continue
		}

		owner := parts[0]
		repo := parts[1]

		// Collect build status for each configured branch
		for _, branchName := range gc.config.GitHub.Branches {
			if err := gc.collectBranchBuildStatus(ctx, owner, repo, branchName); err != nil {
				slog.Error("Failed to collect branch build status", "owner", owner, "repo", repo, "branch", branchName, "error", err)
				gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
					"endpoint":   "build_status",
					"error_type": "branch_error",
				}).Inc()
			}
		}
	}

	return nil
}

// collectBuildStatusForAllRepos collects build status metrics for all accessible repositories
func (gc *GitHubCollector) collectBuildStatusForAllRepos(ctx context.Context) error {
	slog.Debug("Collecting build status metrics for all accessible repositories")

	// Wait for rate limiter
	if err := gc.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}

	// Get all repositories the authenticated user has access to
	repos, resp, err := gc.client.Repositories.ListByAuthenticatedUser(ctx, &github.RepositoryListByAuthenticatedUserOptions{
		Type: "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get repositories: %w", err)
	}

	// Update API call metrics
	if resp != nil {
		gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
			"endpoint": "repos",
			"status":   fmt.Sprintf("%d", resp.StatusCode),
		}).Inc()
	}

	// Collect build status for each repository and branch
	for _, repo := range repos {
		if repo.Owner == nil || repo.Name == nil {
			continue
		}

		owner := *repo.Owner.Login
		repoName := *repo.Name

		// Collect build status for each configured branch
		for _, branchName := range gc.config.GitHub.Branches {
			if err := gc.collectBranchBuildStatus(ctx, owner, repoName, branchName); err != nil {
				slog.Error("Failed to collect branch build status", "owner", owner, "repo", repoName, "branch", branchName, "error", err)
				gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
					"endpoint":   "build_status",
					"error_type": "branch_error",
				}).Inc()
			}
		}
	}

	slog.Debug("Build status metrics collection completed", "repos_processed", len(repos))
	return nil
}

// collectBranchBuildStatus collects build status for a specific branch
func (gc *GitHubCollector) collectBranchBuildStatus(ctx context.Context, owner, repo, branch string) error {
	// Wait for rate limiter
	if err := gc.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}

	// Get workflow runs for the repository (we'll filter by branch in processing)
	workflowRuns, resp, err := gc.client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, &github.ListWorkflowRunsOptions{
		ListOptions: github.ListOptions{
			PerPage: 50, // Get more runs to filter by branch
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get workflow runs for branch %s: %w", branch, err)
	}

	// Update API call metrics
	if resp != nil {
		gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
			"endpoint": "workflow_runs",
			"status":   fmt.Sprintf("%d", resp.StatusCode),
		}).Inc()
	}

	// Process workflow runs
	branchStatus := 1.0 // Default to success
	hasRuns := false

	for _, run := range workflowRuns.WorkflowRuns {
		if run.WorkflowID == nil || run.Name == nil || run.HeadBranch == nil {
			continue
		}

		// Filter by the specific branch we're monitoring
		if *run.HeadBranch != branch {
			continue
		}

		hasRuns = true
		workflowName := *run.Name
		conclusion := "unknown"
		if run.Conclusion != nil {
			conclusion = *run.Conclusion
		}

		// Set workflow run status metric
		statusValue := gc.getStatusValue(conclusion)
		gc.metrics.GitHubWorkflowRunStatus.With(prometheus.Labels{
			"org":        owner,
			"repo":       repo,
			"workflow":   workflowName,
			"branch":     branch,
			"conclusion": conclusion,
		}).Set(statusValue)

		// Set workflow run duration metric
		if run.RunStartedAt != nil && run.UpdatedAt != nil {
			duration := run.UpdatedAt.Sub(run.RunStartedAt.Time).Seconds()
			gc.metrics.GitHubWorkflowRunDuration.With(prometheus.Labels{
				"org":        owner,
				"repo":       repo,
				"workflow":   workflowName,
				"branch":     branch,
				"conclusion": conclusion,
			}).Set(duration)
		}

		// Update branch status (worst status wins)
		if statusValue < branchStatus {
			branchStatus = statusValue
		}
	}

	// Set branch build status metric
	if hasRuns {
		gc.metrics.GitHubBranchBuildStatus.With(prometheus.Labels{
			"org":    owner,
			"repo":   repo,
			"branch": branch,
		}).Set(branchStatus)
	}

	// Get check runs for the branch
	if err := gc.collectCheckRuns(ctx, owner, repo, branch); err != nil {
		slog.Error("Failed to collect check runs", "owner", owner, "repo", repo, "branch", branch, "error", err)
	}

	return nil
}

// collectCheckRuns collects check run status for a specific branch
func (gc *GitHubCollector) collectCheckRuns(ctx context.Context, owner, repo, branch string) error {
	// Wait for rate limiter
	if err := gc.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}

	// Get check runs for the branch
	checkRuns, resp, err := gc.client.Checks.ListCheckRunsForRef(ctx, owner, repo, branch, &github.ListCheckRunsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get check runs for branch %s: %w", branch, err)
	}

	// Update API call metrics
	if resp != nil {
		gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
			"endpoint": "check_runs",
			"status":   fmt.Sprintf("%d", resp.StatusCode),
		}).Inc()
	}

	// Process check runs
	for _, checkRun := range checkRuns.CheckRuns {
		if checkRun.Name == nil {
			continue
		}

		checkName := *checkRun.Name
		conclusion := "unknown"
		if checkRun.Conclusion != nil {
			conclusion = *checkRun.Conclusion
		}

		// Set check run status metric
		statusValue := gc.getStatusValue(conclusion)
		gc.metrics.GitHubCheckRunStatus.With(prometheus.Labels{
			"org":        owner,
			"repo":       repo,
			"check_name": checkName,
			"branch":     branch,
			"conclusion": conclusion,
		}).Set(statusValue)
	}

	return nil
}

// getStatusValue converts GitHub status/conclusion to numeric value
func (gc *GitHubCollector) getStatusValue(conclusion string) float64 {
	switch conclusion {
	case "success":
		return 1.0
	case "failure", "cancelled", "timed_out":
		return 0.0
	case "pending", "in_progress", "queued":
		return 2.0
	case "skipped", "neutral":
		return 3.0
	default:
		return 2.0 // Default to pending for unknown statuses
	}
}

// Stop stops the collector
func (gc *GitHubCollector) Stop() {
	// No cleanup needed for GitHub collector
}
