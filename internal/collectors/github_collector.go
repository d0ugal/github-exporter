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
	var spanCtx context.Context

	if tracer != nil && tracer.IsEnabled() {
		collectorSpan = tracer.NewCollectorSpan(ctx, "github-collector", "collect-metrics")
		collectorSpan.SetAttributes(
			attribute.Int("github.orgs_count", len(gc.config.GitHub.Orgs)),
			attribute.Int("github.repos_count", len(gc.config.GitHub.Repos)),
			attribute.Int("github.branches_count", len(gc.config.GitHub.Branches)),
		)
		spanCtx = collectorSpan.Context()
		defer collectorSpan.End()
	} else {
		spanCtx = ctx
	}

	if collectorSpan != nil {
		collectorSpan.AddEvent("collection_started")
	}

	// Check and update rate limits first
	rateLimitStart := time.Now()
	if err := gc.updateRateLimits(spanCtx); err != nil {
		rateLimitDuration := time.Since(rateLimitStart).Seconds()
		slog.Error("Failed to update rate limits", "error", err)

		if collectorSpan != nil {
			collectorSpan.SetAttributes(
				attribute.Float64("rate_limit.duration_seconds", rateLimitDuration),
			)
			collectorSpan.RecordError(err, attribute.String("operation", "update-rate-limits"))
			collectorSpan.AddEvent("rate_limit_update_failed",
				attribute.String("error", err.Error()),
			)
		}

		gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
			"endpoint":   "rate_limit",
			"error_type": "update_error",
		}).Inc()

		return
	}
	rateLimitDuration := time.Since(rateLimitStart).Seconds()

	if collectorSpan != nil {
		collectorSpan.SetAttributes(
			attribute.Float64("rate_limit.duration_seconds", rateLimitDuration),
		)
		collectorSpan.AddEvent("rate_limit_updated")
	}

	// Wait for rate limiter
	if err := gc.limiter.Wait(spanCtx); err != nil {
		slog.Error("Rate limiter error", "error", err)
		if collectorSpan != nil {
			collectorSpan.RecordError(err, attribute.String("operation", "rate-limiter-wait"))
		}
		return
	}

	// Collect organization metrics
	orgStart := time.Now()
	if err := gc.collectOrgMetrics(spanCtx); err != nil {
		orgDuration := time.Since(orgStart).Seconds()
		slog.Error("Failed to collect organization metrics", "error", err)
		if collectorSpan != nil {
			collectorSpan.SetAttributes(
				attribute.Float64("org_metrics.duration_seconds", orgDuration),
			)
			collectorSpan.RecordError(err, attribute.String("operation", "collect-org-metrics"))
		}
		gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
			"endpoint":   "orgs",
			"error_type": "collection_error",
		}).Inc()
	} else {
		orgDuration := time.Since(orgStart).Seconds()
		if collectorSpan != nil {
			collectorSpan.SetAttributes(
				attribute.Float64("org_metrics.duration_seconds", orgDuration),
			)
			collectorSpan.AddEvent("org_metrics_collected",
				attribute.Float64("duration_seconds", orgDuration),
			)
		}
	}

	// Collect repository metrics
	repoStart := time.Now()
	if err := gc.collectRepoMetrics(spanCtx); err != nil {
		repoDuration := time.Since(repoStart).Seconds()
		slog.Error("Failed to collect repository metrics", "error", err)
		if collectorSpan != nil {
			collectorSpan.SetAttributes(
				attribute.Float64("repo_metrics.duration_seconds", repoDuration),
			)
			collectorSpan.RecordError(err, attribute.String("operation", "collect-repo-metrics"))
		}
		gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
			"endpoint":   "repos",
			"error_type": "collection_error",
		}).Inc()
	} else {
		repoDuration := time.Since(repoStart).Seconds()
		if collectorSpan != nil {
			collectorSpan.SetAttributes(
				attribute.Float64("repo_metrics.duration_seconds", repoDuration),
			)
			collectorSpan.AddEvent("repo_metrics_collected",
				attribute.Float64("duration_seconds", repoDuration),
			)
		}
	}

	// Collect build status metrics if branches are configured
	if len(gc.config.GitHub.Branches) > 0 {
		buildStart := time.Now()
		if err := gc.collectBuildStatusMetrics(spanCtx); err != nil {
			buildDuration := time.Since(buildStart).Seconds()
			slog.Error("Failed to collect build status metrics", "error", err)
			if collectorSpan != nil {
				collectorSpan.SetAttributes(
					attribute.Float64("build_status.duration_seconds", buildDuration),
				)
				collectorSpan.RecordError(err, attribute.String("operation", "collect-build-status"))
			}
			gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
				"endpoint":   "build_status",
				"error_type": "collection_error",
			}).Inc()
		} else {
			buildDuration := time.Since(buildStart).Seconds()
			if collectorSpan != nil {
				collectorSpan.SetAttributes(
					attribute.Float64("build_status.duration_seconds", buildDuration),
				)
				collectorSpan.AddEvent("build_status_collected",
					attribute.Float64("duration_seconds", buildDuration),
				)
			}
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
	tracer := gc.app.GetTracer()

	var collectorSpan *tracing.CollectorSpan
	var spanCtx context.Context

	if tracer != nil && tracer.IsEnabled() {
		collectorSpan = tracer.NewCollectorSpan(ctx, "github-collector", "update-rate-limits")
		spanCtx = collectorSpan.Context()
		defer collectorSpan.End()
	} else {
		spanCtx = ctx
	}

	// Only check rate limits if it's been more than 5 minutes since last check
	// or if we've never checked before
	gc.mu.RLock()
	lastCheck := gc.lastRateLimitCheck
	gc.mu.RUnlock()

	if time.Since(lastCheck) < 5*time.Minute && !lastCheck.IsZero() {
		if collectorSpan != nil {
			collectorSpan.AddEvent("rate_limit_check_skipped",
				attribute.String("reason", "recent_check"),
			)
		}
		return nil // Skip rate limit check
	}

	// Wait for rate limiter
	waitStart := time.Now()
	if err := gc.limiter.Wait(spanCtx); err != nil {
		if collectorSpan != nil {
			collectorSpan.RecordError(err, attribute.String("operation", "rate-limiter-wait"))
		}
		return fmt.Errorf("rate limiter error: %w", err)
	}
	waitDuration := time.Since(waitStart).Seconds()

	// Get rate limit information using the new API
	apiStart := time.Now()
	rateLimit, resp, err := gc.client.RateLimit.Get(spanCtx)
	apiDuration := time.Since(apiStart).Seconds()

	if err != nil {
		if collectorSpan != nil {
			collectorSpan.SetAttributes(
				attribute.Float64("api_call.duration_seconds", apiDuration),
			)
			collectorSpan.RecordError(err, attribute.String("operation", "get-rate-limit"))
		}
		return fmt.Errorf("failed to get rate limit info: %w", err)
	}

	// Update API call metrics
	gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
		"endpoint": "rate_limit",
		"status":   fmt.Sprintf("%d", resp.StatusCode),
	}).Inc()

	// Update rate limit state
	gc.mu.Lock()
	var limit, remaining int
	var resetTime time.Time
	if rateLimit.Core != nil {
		gc.rateLimitTotal = rateLimit.Core.Limit
		gc.rateLimitRemaining = rateLimit.Core.Remaining
		limit = rateLimit.Core.Limit
		remaining = rateLimit.Core.Remaining
		if !rateLimit.Core.Reset.IsZero() {
			gc.rateLimitReset = rateLimit.Core.Reset.Time
			resetTime = rateLimit.Core.Reset.Time
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
	limiterStart := time.Now()
	gc.updateRateLimiter()
	limiterDuration := time.Since(limiterStart).Seconds()

	if collectorSpan != nil {
		collectorSpan.SetAttributes(
			attribute.Float64("rate_limiter_wait.duration_seconds", waitDuration),
			attribute.Float64("api_call.duration_seconds", apiDuration),
			attribute.Float64("rate_limiter_update.duration_seconds", limiterDuration),
			attribute.Int("rate_limit.total", limit),
			attribute.Int("rate_limit.remaining", remaining),
		)
		if !resetTime.IsZero() {
			collectorSpan.SetAttributes(
				attribute.Int64("rate_limit.reset_timestamp", resetTime.Unix()),
			)
		}
		collectorSpan.AddEvent("rate_limit_updated",
			attribute.Int("total", limit),
			attribute.Int("remaining", remaining),
		)
	}

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
	tracer := gc.app.GetTracer()

	var collectorSpan *tracing.CollectorSpan
	var spanCtx context.Context

	if tracer != nil && tracer.IsEnabled() {
		collectorSpan = tracer.NewCollectorSpan(ctx, "github-collector", "collect-org-metrics")
		collectorSpan.SetAttributes(
			attribute.Int("orgs.count", len(gc.config.GitHub.Orgs)),
		)
		spanCtx = collectorSpan.Context()
		defer collectorSpan.End()
	} else {
		spanCtx = ctx
	}

	successCount := 0
	errorCount := 0

	// Collect metrics for each organization
	for _, org := range gc.config.GitHub.Orgs {
		orgStart := time.Now()

		// Wait for rate limiter
		if err := gc.limiter.Wait(spanCtx); err != nil {
			if collectorSpan != nil {
				collectorSpan.RecordError(err, attribute.String("org", org), attribute.String("operation", "rate-limiter-wait"))
			}
			errorCount++
			continue
		}

		// Get organization information
		apiStart := time.Now()
		orgInfo, resp, err := gc.client.Organizations.Get(spanCtx, org)
		apiDuration := time.Since(apiStart).Seconds()

		if err != nil {
			slog.Error("Failed to get organization info", "org", org, "error", err)
			if collectorSpan != nil {
				collectorSpan.SetAttributes(
					attribute.Float64("org.api_duration_seconds", apiDuration),
				)
				collectorSpan.RecordError(err, attribute.String("org", org), attribute.String("operation", "get-org-info"))
			}
			gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
				"endpoint":   "orgs",
				"error_type": "api_error",
			}).Inc()
			errorCount++
			// Skip this org entirely - don't collect repos for a non-existent org
			continue
		}

		// Update API call metrics
		statusCode := "unknown"
		if resp != nil {
			statusCode = fmt.Sprintf("%d", resp.StatusCode)
		}
		gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
			"endpoint": "orgs",
			"status":   statusCode,
		}).Inc()

		// Check for 404 even if err is nil (some APIs return status without error)
		if resp != nil && resp.StatusCode == 404 {
			slog.Warn("Organization not found (404), skipping", "org", org)
			// Skip this org entirely - don't collect repos for a non-existent org
			continue
		}

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

		orgDuration := time.Since(orgStart).Seconds()

		if collectorSpan != nil {
			collectorSpan.AddEvent("org_info_retrieved",
				attribute.String("org", org),
				attribute.Float64("duration_seconds", orgDuration),
			)
		}

		// Get repositories for the organization
		// Only collect repos if org fetch was successful
		reposStart := time.Now()
		if err := gc.collectOrgRepos(spanCtx, org); err != nil {
			reposDuration := time.Since(reposStart).Seconds()
			slog.Error("Failed to collect organization repositories", "org", org, "error", err)
			if collectorSpan != nil {
				collectorSpan.SetAttributes(
					attribute.Float64("org.repos_duration_seconds", reposDuration),
				)
				collectorSpan.RecordError(err, attribute.String("org", org), attribute.String("operation", "collect-org-repos"))
			}
			errorCount++
			// Continue to next org instead of failing completely
			continue
		}
		reposDuration := time.Since(reposStart).Seconds()
		totalOrgDuration := time.Since(orgStart).Seconds()

		if collectorSpan != nil {
			collectorSpan.SetAttributes(
				attribute.Float64("org.total_duration_seconds", totalOrgDuration),
				attribute.Float64("org.repos_duration_seconds", reposDuration),
			)
			collectorSpan.AddEvent("org_collected",
				attribute.String("org", org),
				attribute.Float64("total_duration_seconds", totalOrgDuration),
			)
		}

		successCount++
	}

	// Set total organizations count
	gc.metrics.GitHubOrgsTotal.With(prometheus.Labels{}).Set(float64(len(gc.config.GitHub.Orgs)))

	if collectorSpan != nil {
		collectorSpan.SetAttributes(
			attribute.Int("collection.successful", successCount),
			attribute.Int("collection.errors", errorCount),
			attribute.Int("collection.total", len(gc.config.GitHub.Orgs)),
		)
		collectorSpan.AddEvent("org_metrics_completed",
			attribute.Int("successful", successCount),
			attribute.Int("errors", errorCount),
		)
	}

	return nil
}

func (gc *GitHubCollector) collectOrgRepos(ctx context.Context, org string) error {
	tracer := gc.app.GetTracer()

	var collectorSpan *tracing.CollectorSpan
	var spanCtx context.Context

	if tracer != nil && tracer.IsEnabled() {
		collectorSpan = tracer.NewCollectorSpan(ctx, "github-collector", "collect-org-repos")
		collectorSpan.SetAttributes(
			attribute.String("org", org),
		)
		spanCtx = collectorSpan.Context()
		defer collectorSpan.End()
	} else {
		spanCtx = ctx
	}

	// Validate org parameter
	if org == "" {
		err := fmt.Errorf("org parameter cannot be empty")
		if collectorSpan != nil {
			collectorSpan.RecordError(err)
		}
		return err
	}

	// Wait for rate limiter
	if err := gc.limiter.Wait(spanCtx); err != nil {
		if collectorSpan != nil {
			collectorSpan.RecordError(err, attribute.String("operation", "rate-limiter-wait"))
		}
		return fmt.Errorf("rate limiter error: %w", err)
	}

	// List repositories for the organization
	apiStart := time.Now()
	repos, resp, err := gc.client.Repositories.ListByOrg(spanCtx, org, &github.RepositoryListByOrgOptions{
		Type: "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	})
	apiDuration := time.Since(apiStart).Seconds()

	if err != nil {
		if collectorSpan != nil {
			collectorSpan.SetAttributes(
				attribute.Float64("api_call.duration_seconds", apiDuration),
			)
			collectorSpan.RecordError(err, attribute.String("operation", "list-repos-by-org"))
		}
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

	// Set repository counts (double-check org is not empty before setting metrics)
	if org == "" {
		err := fmt.Errorf("org parameter is empty when setting repo counts")
		slog.Error("Cannot set GitHubReposTotal: org is empty")
		if collectorSpan != nil {
			collectorSpan.RecordError(err)
		}
		return err
	}
	gc.metrics.GitHubReposTotal.With(prometheus.Labels{
		"org":        org,
		"visibility": "public",
	}).Set(float64(publicCount))
	gc.metrics.GitHubReposTotal.With(prometheus.Labels{
		"org":        org,
		"visibility": "private",
	}).Set(float64(privateCount))

	if collectorSpan != nil {
		collectorSpan.SetAttributes(
			attribute.Float64("api_call.duration_seconds", apiDuration),
			attribute.Int("repos.total", len(repos)),
			attribute.Int("repos.public", publicCount),
			attribute.Int("repos.private", privateCount),
		)
		collectorSpan.AddEvent("org_repos_collected",
			attribute.Int("total", len(repos)),
			attribute.Int("public", publicCount),
			attribute.Int("private", privateCount),
		)
	}

	return nil
}

func (gc *GitHubCollector) collectRepoMetrics(ctx context.Context) error {
	tracer := gc.app.GetTracer()

	var collectorSpan *tracing.CollectorSpan
	var spanCtx context.Context

	if tracer != nil && tracer.IsEnabled() {
		collectorSpan = tracer.NewCollectorSpan(ctx, "github-collector", "collect-repo-metrics")
		collectorSpan.SetAttributes(
			attribute.Int("repos.count", len(gc.config.GitHub.Repos)),
			attribute.Bool("repos.wildcard", gc.hasWildcardRepos()),
		)
		spanCtx = collectorSpan.Context()
		defer collectorSpan.End()
	} else {
		spanCtx = ctx
	}

	// Check if wildcard is specified for repos
	if gc.hasWildcardRepos() {
		if collectorSpan != nil {
			collectorSpan.AddEvent("wildcard_repos_detected")
		}
		return gc.collectAllRepos(spanCtx)
	}

	successCount := 0
	errorCount := 0

	// Collect metrics for specific repositories
	for _, repoFullName := range gc.config.GitHub.Repos {
		repoStart := time.Now()

		parts := strings.Split(repoFullName, "/")
		if len(parts) != 2 {
			slog.Error("Invalid repository format", "repo", repoFullName)
			errorCount++
			continue
		}

		owner := parts[0]
		repo := parts[1]

		// Skip if owner or repo is empty
		if owner == "" || repo == "" {
			slog.Error("Invalid repository format: owner or repo is empty", "repo", repoFullName)
			errorCount++
			continue
		}

		// Wait for rate limiter
		if err := gc.limiter.Wait(spanCtx); err != nil {
			if collectorSpan != nil {
				collectorSpan.RecordError(err, attribute.String("repo", repoFullName), attribute.String("operation", "rate-limiter-wait"))
			}
			errorCount++
			continue
		}

		// Get repository information
		apiStart := time.Now()
		repoInfo, resp, err := gc.client.Repositories.Get(spanCtx, owner, repo)
		apiDuration := time.Since(apiStart).Seconds()

		if err != nil {
			repoDuration := time.Since(repoStart).Seconds()
			slog.Error("Failed to get repository info", "owner", owner, "repo", repo, "error", err)
			if collectorSpan != nil {
				collectorSpan.SetAttributes(
					attribute.Float64("repo.api_duration_seconds", apiDuration),
					attribute.Float64("repo.total_duration_seconds", repoDuration),
				)
				collectorSpan.RecordError(err, attribute.String("repo", repoFullName), attribute.String("operation", "get-repo-info"))
			}
			gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
				"endpoint":   "repos",
				"error_type": "api_error",
			}).Inc()
			errorCount++
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

		metricsStart := time.Now()
		gc.setRepoMetrics(spanCtx, owner, repo, visibility, repoInfo)
		metricsDuration := time.Since(metricsStart).Seconds()
		repoDuration := time.Since(repoStart).Seconds()

		if collectorSpan != nil {
			collectorSpan.SetAttributes(
				attribute.Float64("repo.api_duration_seconds", apiDuration),
				attribute.Float64("repo.metrics_duration_seconds", metricsDuration),
				attribute.Float64("repo.total_duration_seconds", repoDuration),
			)
			collectorSpan.AddEvent("repo_collected",
				attribute.String("repo", repoFullName),
				attribute.String("visibility", visibility),
			)
		}

		successCount++
	}

	if collectorSpan != nil {
		collectorSpan.SetAttributes(
			attribute.Int("collection.successful", successCount),
			attribute.Int("collection.errors", errorCount),
			attribute.Int("collection.total", len(gc.config.GitHub.Repos)),
		)
		collectorSpan.AddEvent("repo_metrics_completed",
			attribute.Int("successful", successCount),
			attribute.Int("errors", errorCount),
		)
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

	// Use GitHub Search API to get exact count of open pull requests
	query := fmt.Sprintf("repo:%s/%s type:pr state:open", owner, repo)
	searchResult, resp, err := gc.client.Search.Issues(ctx, query, &github.SearchOptions{
		ListOptions: github.ListOptions{
			PerPage: 1, // We only need the count, not the actual PRs
		},
	})
	if err != nil {
		slog.Error("Failed to search open PRs", "owner", owner, "repo", repo, "error", err)
		gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
			"endpoint":   "search_issues",
			"error_type": "api_error",
		}).Inc()
		return
	}

	// Update API call metrics
	if resp != nil {
		gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
			"endpoint": "search_issues",
			"status":   fmt.Sprintf("%d", resp.StatusCode),
		}).Inc()
	}

	// Get the exact count from search results
	openPRsCount := 0
	if searchResult != nil && searchResult.Total != nil {
		openPRsCount = *searchResult.Total
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

	var allRepos []*github.Repository
	page := 1
	perPage := 100

	for {
		// Wait for rate limiter
		if err := gc.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter error: %w", err)
		}

		// Get repositories for current page
		repos, resp, err := gc.client.Repositories.ListByAuthenticatedUser(ctx, &github.RepositoryListByAuthenticatedUserOptions{
			Type: "all",
			ListOptions: github.ListOptions{
				Page:    page,
				PerPage: perPage,
			},
		})
		if err != nil {
			slog.Error("Failed to list repositories", "page", page, "error", err)
			gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
				"endpoint":   "repos",
				"error_type": "api_error",
			}).Inc()
			return fmt.Errorf("failed to list repositories page %d: %w", page, err)
		}

		// Update API call metrics
		if resp != nil {
			gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
				"endpoint": "repos",
				"status":   fmt.Sprintf("%d", resp.StatusCode),
			}).Inc()
		}

		// Add repos to our collection
		allRepos = append(allRepos, repos...)

		// Check if we've reached the last page
		if resp == nil || page >= resp.LastPage || len(repos) < perPage {
			break
		}

		page++
	}

	// Process each repository
	for _, repo := range allRepos {
		if repo == nil || repo.Name == nil || repo.Owner == nil || repo.Owner.Login == nil {
			slog.Warn("Skipping repository with missing required fields", "repo", repo)
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

	slog.Info("Collected metrics for repositories", "count", len(allRepos))

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

	var allRepos []*github.Repository
	page := 1
	perPage := 100

	for {
		// Wait for rate limiter
		if err := gc.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter error: %w", err)
		}

		// Get repositories for current page
		repos, resp, err := gc.client.Repositories.ListByAuthenticatedUser(ctx, &github.RepositoryListByAuthenticatedUserOptions{
			Type: "all",
			ListOptions: github.ListOptions{
				Page:    page,
				PerPage: perPage,
			},
		})
		if err != nil {
			slog.Error("Failed to list repositories", "page", page, "error", err)
			gc.metrics.GitHubAPIErrorsTotal.With(prometheus.Labels{
				"endpoint":   "repos",
				"error_type": "api_error",
			}).Inc()
			return fmt.Errorf("failed to list repositories page %d: %w", page, err)
		}

		// Update API call metrics
		if resp != nil {
			gc.metrics.GitHubAPICallsTotal.With(prometheus.Labels{
				"endpoint": "repos",
				"status":   fmt.Sprintf("%d", resp.StatusCode),
			}).Inc()
		}

		// Add repos to our collection
		allRepos = append(allRepos, repos...)

		// Check if we've reached the last page
		if resp == nil || page >= resp.LastPage || len(repos) < perPage {
			break
		}

		page++
	}

	// Collect build status for each repository and branch
	for _, repo := range allRepos {
		if repo == nil || repo.Owner == nil || repo.Name == nil {
			slog.Warn("Skipping repository with missing required fields for build status", "repo", repo)
			continue
		}

		owner := *repo.Owner.Login
		repoName := *repo.Name

		// Skip if owner or repo name is empty
		if owner == "" || repoName == "" {
			slog.Warn("Skipping repository with empty owner or name for build status", "owner", owner, "repo", repoName)
			continue
		}

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

	slog.Debug("Build status metrics collection completed", "repos_processed", len(allRepos))
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
