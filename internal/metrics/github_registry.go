package metrics

import (
	promexporter_metrics "github.com/d0ugal/promexporter/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// GitHubRegistry wraps the promexporter registry with GitHub-specific metrics
type GitHubRegistry struct {
	*promexporter_metrics.Registry

	// GitHub repository metrics
	GitHubReposTotal       *prometheus.GaugeVec
	GitHubReposInfo        *prometheus.GaugeVec
	GitHubReposStars       *prometheus.GaugeVec
	GitHubReposForks       *prometheus.GaugeVec
	GitHubReposWatchers    *prometheus.GaugeVec
	GitHubReposOpenIssues  *prometheus.GaugeVec
	GitHubReposOpenPRs     *prometheus.GaugeVec
	GitHubReposSize        *prometheus.GaugeVec
	GitHubReposLastUpdated *prometheus.GaugeVec
	GitHubReposCreatedAt   *prometheus.GaugeVec

	// GitHub organization metrics
	GitHubOrgsTotal       *prometheus.GaugeVec
	GitHubOrgsPublicRepos *prometheus.GaugeVec
	GitHubOrgsFollowers   *prometheus.GaugeVec
	GitHubOrgsFollowing   *prometheus.GaugeVec

	// GitHub build status metrics
	GitHubBranchBuildStatus   *prometheus.GaugeVec
	GitHubWorkflowRunStatus   *prometheus.GaugeVec
	GitHubCheckRunStatus      *prometheus.GaugeVec
	GitHubWorkflowRunDuration *prometheus.GaugeVec

	// GitHub API metrics
	GitHubAPICallsTotal      *prometheus.CounterVec
	GitHubAPIErrorsTotal     *prometheus.CounterVec
	GitHubRateLimitTotal     *prometheus.GaugeVec
	GitHubRateLimitRemaining *prometheus.GaugeVec
	GitHubRateLimitReset     *prometheus.GaugeVec
}

// NewGitHubRegistry creates a new GitHub metrics registry
func NewGitHubRegistry(baseRegistry *promexporter_metrics.Registry) *GitHubRegistry {
	// Get the underlying Prometheus registry
	promRegistry := baseRegistry.GetRegistry()
	factory := promauto.With(promRegistry)

	github := &GitHubRegistry{
		Registry: baseRegistry,
	}

	// GitHub repository metrics
	github.GitHubReposTotal = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_repos_total",
			Help: "Total number of GitHub repositories",
		},
		[]string{"org", "visibility"},
	)
	baseRegistry.AddMetricInfo("github_repos_total", "Total number of GitHub repositories", []string{"org", "visibility"})

	github.GitHubReposInfo = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_repo_info",
			Help: "Information about GitHub repositories",
		},
		[]string{"org", "repo", "visibility", "archived", "fork", "language"},
	)
	baseRegistry.AddMetricInfo("github_repo_info", "Information about GitHub repositories", []string{"org", "repo", "visibility", "archived", "fork", "language"})

	github.GitHubReposStars = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_repo_stars",
			Help: "Number of stars for a GitHub repository",
		},
		[]string{"org", "repo", "visibility"},
	)
	baseRegistry.AddMetricInfo("github_repo_stars", "Number of stars for a GitHub repository", []string{"org", "repo", "visibility"})

	github.GitHubReposForks = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_repo_forks",
			Help: "Number of forks for a GitHub repository",
		},
		[]string{"org", "repo", "visibility"},
	)
	baseRegistry.AddMetricInfo("github_repo_forks", "Number of forks for a GitHub repository", []string{"org", "repo", "visibility"})

	github.GitHubReposWatchers = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_repo_watchers",
			Help: "Number of watchers for a GitHub repository",
		},
		[]string{"org", "repo", "visibility"},
	)
	baseRegistry.AddMetricInfo("github_repo_watchers", "Number of watchers for a GitHub repository", []string{"org", "repo", "visibility"})

	github.GitHubReposOpenIssues = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_repo_open_issues",
			Help: "Number of open issues for a GitHub repository",
		},
		[]string{"org", "repo", "visibility"},
	)
	baseRegistry.AddMetricInfo("github_repo_open_issues", "Number of open issues for a GitHub repository", []string{"org", "repo", "visibility"})

	github.GitHubReposOpenPRs = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_repo_open_prs",
			Help: "Number of open pull requests for a GitHub repository",
		},
		[]string{"org", "repo", "visibility"},
	)
	baseRegistry.AddMetricInfo("github_repo_open_prs", "Number of open pull requests for a GitHub repository", []string{"org", "repo", "visibility"})

	github.GitHubReposSize = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_repo_size_bytes",
			Help: "Size of a GitHub repository in bytes",
		},
		[]string{"org", "repo", "visibility"},
	)
	baseRegistry.AddMetricInfo("github_repo_size_bytes", "Size of a GitHub repository in bytes", []string{"org", "repo", "visibility"})

	github.GitHubReposLastUpdated = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_repo_last_updated_timestamp",
			Help: "Unix timestamp of the last update for a GitHub repository",
		},
		[]string{"org", "repo", "visibility"},
	)
	baseRegistry.AddMetricInfo("github_repo_last_updated_timestamp", "Unix timestamp of the last update for a GitHub repository", []string{"org", "repo", "visibility"})

	github.GitHubReposCreatedAt = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_repo_created_timestamp",
			Help: "Unix timestamp of the creation date for a GitHub repository",
		},
		[]string{"org", "repo", "visibility"},
	)
	baseRegistry.AddMetricInfo("github_repo_created_timestamp", "Unix timestamp of the creation date for a GitHub repository", []string{"org", "repo", "visibility"})

	// GitHub organization metrics
	github.GitHubOrgsTotal = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_orgs_total",
			Help: "Total number of GitHub organizations",
		},
		[]string{},
	)
	baseRegistry.AddMetricInfo("github_orgs_total", "Total number of GitHub organizations", []string{})

	github.GitHubOrgsPublicRepos = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_org_public_repos",
			Help: "Number of public repositories for a GitHub organization",
		},
		[]string{"org"},
	)
	baseRegistry.AddMetricInfo("github_org_public_repos", "Number of public repositories for a GitHub organization", []string{"org"})

	github.GitHubOrgsFollowers = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_org_followers",
			Help: "Number of followers for a GitHub organization",
		},
		[]string{"org"},
	)
	baseRegistry.AddMetricInfo("github_org_followers", "Number of followers for a GitHub organization", []string{"org"})

	github.GitHubOrgsFollowing = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_org_following",
			Help: "Number of organizations that a GitHub organization is following",
		},
		[]string{"org"},
	)
	baseRegistry.AddMetricInfo("github_org_following", "Number of organizations that a GitHub organization is following", []string{"org"})

	// GitHub build status metrics
	github.GitHubBranchBuildStatus = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_branch_build_status",
			Help: "Build status for GitHub repository branches (0=failed, 1=success, 2=pending, 3=skipped)",
		},
		[]string{"org", "repo", "branch"},
	)
	baseRegistry.AddMetricInfo("github_branch_build_status", "Build status for GitHub repository branches (0=failed, 1=success, 2=pending, 3=skipped)", []string{"org", "repo", "branch"})

	github.GitHubWorkflowRunStatus = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_workflow_run_status",
			Help: "Status of GitHub workflow runs (0=failed, 1=success, 2=pending, 3=skipped)",
		},
		[]string{"org", "repo", "workflow", "branch", "conclusion"},
	)
	baseRegistry.AddMetricInfo("github_workflow_run_status", "Status of GitHub workflow runs (0=failed, 1=success, 2=pending, 3=skipped)", []string{"org", "repo", "workflow", "branch", "conclusion"})

	github.GitHubCheckRunStatus = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_check_run_status",
			Help: "Status of GitHub check runs (0=failed, 1=success, 2=pending, 3=skipped)",
		},
		[]string{"org", "repo", "check_name", "branch", "conclusion"},
	)
	baseRegistry.AddMetricInfo("github_check_run_status", "Status of GitHub check runs (0=failed, 1=success, 2=pending, 3=skipped)", []string{"org", "repo", "check_name", "branch", "conclusion"})

	github.GitHubWorkflowRunDuration = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_workflow_run_duration_seconds",
			Help: "Duration of GitHub workflow runs in seconds",
		},
		[]string{"org", "repo", "workflow", "branch", "conclusion"},
	)
	baseRegistry.AddMetricInfo("github_workflow_run_duration_seconds", "Duration of GitHub workflow runs in seconds", []string{"org", "repo", "workflow", "branch", "conclusion"})

	// GitHub API metrics
	github.GitHubAPICallsTotal = factory.NewCounterVec(
		prometheus.CounterOpts{
			Name: "github_api_calls_total",
			Help: "Total number of GitHub API calls made",
		},
		[]string{"endpoint", "status"},
	)
	baseRegistry.AddMetricInfo("github_api_calls_total", "Total number of GitHub API calls made", []string{"endpoint", "status"})

	github.GitHubAPIErrorsTotal = factory.NewCounterVec(
		prometheus.CounterOpts{
			Name: "github_api_errors_total",
			Help: "Total number of GitHub API errors",
		},
		[]string{"endpoint", "error_type"},
	)
	baseRegistry.AddMetricInfo("github_api_errors_total", "Total number of GitHub API errors", []string{"endpoint", "error_type"})

	github.GitHubRateLimitTotal = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_rate_limit_total",
			Help: "Total number of GitHub API requests allowed in the current rate limit window",
		},
		[]string{},
	)
	baseRegistry.AddMetricInfo("github_rate_limit_total", "Total number of GitHub API requests allowed in the current rate limit window", []string{})

	github.GitHubRateLimitRemaining = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_rate_limit_remaining",
			Help: "Number of GitHub API requests remaining in the current rate limit window",
		},
		[]string{},
	)
	baseRegistry.AddMetricInfo("github_rate_limit_remaining", "Number of GitHub API requests remaining in the current rate limit window", []string{})

	github.GitHubRateLimitReset = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_rate_limit_reset_timestamp",
			Help: "Unix timestamp when the GitHub API rate limit resets",
		},
		[]string{},
	)
	baseRegistry.AddMetricInfo("github_rate_limit_reset_timestamp", "Unix timestamp when the GitHub API rate limit resets", []string{})

	return github
}
