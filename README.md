# GitHub Exporter

A Prometheus exporter for GitHub metrics that collects repository statistics, organization data, and user information from the GitHub API.

## Features

- **Repository Metrics**: Stars, forks, issues, pull requests, and more
- **Organization Monitoring**: Track multiple organizations and their repositories
- **Build Status Monitoring**: Track build status, workflow runs, and check runs for specific branches
- **Rate Limit Management**: Intelligent rate limiting to respect GitHub API limits
- **Flexible Configuration**: YAML config file or environment variables
- **Docker Support**: Ready-to-use Docker container
- **Prometheus Integration**: Native Prometheus metrics format

## Quick Start

### Using Docker

```bash
# Run with environment variables
docker run -d \
  --name github-exporter \
  -p 8080:8080 \
  -e GITHUB_EXPORTER_GITHUB_TOKEN=your_token_here \
  -e GITHUB_EXPORTER_GITHUB_REPOS="d0ugal/mqtt-exporter,d0ugal/filesystem-exporter" \
  ghcr.io/d0ugal/github-exporter:latest
```

### Using Configuration File

1. Copy the example configuration:
```bash
cp config.example.yaml config.yaml
```

2. Edit `config.yaml` with your GitHub token and repositories:
```yaml
github:
  token: "ghp_your_token_here"
  repos:
    - "d0ugal/mqtt-exporter"
    - "d0ugal/filesystem-exporter"
```

3. Run the exporter:
```bash
./github-exporter
```

## Configuration

### GitHub Token

You need a GitHub Personal Access Token with appropriate permissions:
- `repo` (for private repositories)
- `read:org` (for organization data)
- `read:user` (for user information)

### Configuration Options

#### YAML Configuration

```yaml
# Server configuration
server:
  host: "0.0.0.0"
  port: 8080

# Logging configuration
logging:
  level: "info"
  format: "json"

# Metrics configuration
metrics:
  collection:
    default_interval: 30s

# GitHub configuration
github:
  token: "ghp_your_token_here"
  
  # Organizations to monitor
  orgs:
    - "d0ugal"
    - "prometheus"
  
  # Specific repositories to monitor
  repos:
    - "d0ugal/mqtt-exporter"
    - "d0ugal/filesystem-exporter"
    # - "*"  # Monitor ALL accessible repositories
  
  # Branches to monitor for build status (optional)
  branches:
    - "main"
    - "develop"
    - "feature/new-feature"
  
  # Specific workflows to monitor (optional, empty = all workflows)
  workflows:
    - "CI"
    - "CD"
  
  # API settings
  timeout: 30s
  refresh_interval: 0s  # Auto-calculate based on rate limits
  rate_limit_buffer: 0.8  # Use 80% of available rate limit
```

#### Environment Variables

All configuration can be set via environment variables:

```bash
GITHUB_EXPORTER_SERVER_HOST=0.0.0.0
GITHUB_EXPORTER_SERVER_PORT=8080
GITHUB_EXPORTER_LOG_LEVEL=info
GITHUB_EXPORTER_LOG_FORMAT=json
GITHUB_EXPORTER_METRICS_DEFAULT_INTERVAL=30s
GITHUB_EXPORTER_GITHUB_TOKEN=ghp_your_token_here
GITHUB_EXPORTER_GITHUB_ORGS=d0ugal,prometheus
GITHUB_EXPORTER_GITHUB_REPOS=d0ugal/mqtt-exporter,d0ugal/filesystem-exporter
GITHUB_EXPORTER_GITHUB_BRANCHES=main,develop
GITHUB_EXPORTER_GITHUB_WORKFLOWS=CI,CD
GITHUB_EXPORTER_GITHUB_TIMEOUT=30s
GITHUB_EXPORTER_GITHUB_RATE_LIMIT=0.8
```

## Metrics

The exporter provides the following metrics:

### Repository Metrics
- `github_repository_stars_total` - Total number of stars
- `github_repository_forks_total` - Total number of forks
- `github_repository_issues_open` - Number of open issues
- `github_repository_issues_closed` - Number of closed issues
- `github_repository_pull_requests_open` - Number of open pull requests
- `github_repository_pull_requests_closed` - Number of closed pull requests
- `github_repository_size_bytes` - Repository size in bytes
- `github_repository_watchers_total` - Number of watchers

### Organization Metrics
- `github_organization_public_repos` - Number of public repositories
- `github_organization_total_repos` - Total number of repositories
- `github_organization_members_total` - Number of organization members

### Build Status Metrics
- `github_branch_build_status` - Build status for repository branches (0=failed, 1=success, 2=pending, 3=skipped)
- `github_workflow_run_status` - Status of workflow runs (0=failed, 1=success, 2=pending, 3=skipped)
- `github_workflow_run_duration_seconds` - Duration of workflow runs in seconds
- `github_check_run_status` - Status of check runs (0=failed, 1=success, 2=pending, 3=skipped)

### Rate Limiting Metrics
- `github_rate_limit_remaining` - Remaining API calls
- `github_rate_limit_limit` - Total API call limit
- `github_rate_limit_reset` - Rate limit reset timestamp

## Development

### Prerequisites

- Go 1.25+
- Docker (for containerized builds)
- Make (for build automation)

### Building

```bash
# Build the application
make build

# Run tests
make test

# Format code
make fmt

# Run linting
make lint

# Clean build artifacts
make clean
```

### Docker Build

```bash
# Build Docker image
docker build -t github-exporter .

# Run with Docker Compose
docker-compose up
```

## API Endpoints

- `GET /metrics` - Prometheus metrics endpoint
- `GET /health` - Health check endpoint
- `GET /version` - Version information

## Build Status Monitoring

The exporter can monitor build status for specific branches by tracking:

- **Workflow Runs**: GitHub Actions workflow execution status and duration
- **Check Runs**: Status checks, CI/CD pipeline results, and external integrations
- **Branch Status**: Overall build health per branch (worst status wins)

### Configuration

To enable build status monitoring, configure the `branches` option:

```yaml
github:
  repos:
    - "d0ugal/mqtt-exporter"
  branches:
    - "main"
    - "develop"
    - "feature/new-feature"
```

### Status Values

Build status metrics use numeric values for easy alerting:

- `0` = Failed (failure, cancelled, timed_out)
- `1` = Success
- `2` = Pending (pending, in_progress, queued)
- `3` = Skipped (skipped, neutral)

### Example Queries

```promql
# Alert on failed builds
github_branch_build_status == 0

# Monitor workflow run durations
github_workflow_run_duration_seconds{conclusion="success"}

# Track check run failures
github_check_run_status == 0
```

## Rate Limiting

The exporter automatically manages GitHub API rate limits:

- Fetches current rate limit status from GitHub API
- Calculates optimal refresh intervals
- Respects rate limit buffers to avoid hitting limits
- Provides rate limit metrics for monitoring

## PromQL Examples with `group_left`

The GitHub exporter provides rich metrics that can be combined using PromQL's `group_left` operator to create powerful queries. Here are some common examples:

### Basic Repository Filtering

Filter repositories by organization and exclude archived/forks:
```promql
# Only active repositories (not archived, not forks)
github_repo_open_issues{org="d0ugal"} 
* on(org,repo) group_left() 
github_repo_info{org="d0ugal", archived="false", fork="false"}
```

### Repository Health Monitoring

Monitor repository health with multiple metrics:
```promql
# Repository health score (lower is better)
(
  github_repo_open_issues{org="d0ugal"} * 2 +
  github_repo_open_prs{org="d0ugal"} * 1
) 
* on(org,repo) group_left() 
github_repo_info{org="d0ugal", archived="false", fork="false"}
```

### Language-Specific Analysis

Analyze metrics by programming language:
```promql
# Open issues by language
github_repo_open_issues{org="d0ugal"} 
* on(org,repo) group_left(language) 
github_repo_info{org="d0ugal", archived="false", fork="false"}
```

### Repository Activity Trends

Track repository activity over time:
```promql
# Rate of new issues per day
rate(github_repo_open_issues{org="d0ugal"}[24h]) 
* on(org,repo) group_left() 
github_repo_info{org="d0ugal", archived="false", fork="false"}
```

### Multi-Organization Monitoring

Monitor across multiple organizations:
```promql
# Issues across all monitored organizations
github_repo_open_issues{org=~"d0ugal|prometheus|kubernetes"} 
* on(org,repo) group_left() 
github_repo_info{archived="false", fork="false"}
```

### Repository Size vs Activity

Correlate repository size with activity:
```promql
# Activity density (issues per MB)
github_repo_open_issues{org="d0ugal"} 
/ on(org,repo) group_left() 
(github_repo_size_bytes{org="d0ugal"} / 1024 / 1024)
```

### Advanced Filtering Examples

```promql
# Only Go repositories with high activity
github_repo_open_issues{org="d0ugal"} 
* on(org,repo) group_left(language) 
github_repo_info{org="d0ugal", language="Go", archived="false", fork="false"}

# Public repositories only
github_repo_open_prs{org="d0ugal"} 
* on(org,repo) group_left(visibility) 
github_repo_info{org="d0ugal", visibility="public", archived="false"}

# Recently active repositories (last 7 days)
github_repo_open_issues{org="d0ugal"} 
* on(org,repo) group_left() 
github_repo_info{org="d0ugal", archived="false", fork="false"}
```

### Understanding `group_left`

The `group_left` operator is crucial for combining metrics with different label sets:

- **Left side**: The metric you want to keep (e.g., `github_repo_open_issues`)
- **Right side**: The metric providing additional labels (e.g., `github_repo_info`)
- **Result**: All labels from both sides, with the right side's labels added to the left

This allows you to:
- Filter repositories by metadata (archived, fork, language, visibility)
- Add context to metrics (language, organization type)
- Create complex queries that combine multiple data sources

## Monitoring

Monitor the exporter itself:

- `github_exporter_info` - Exporter information
- `github_exporter_up` - Exporter health status
- `github_exporter_scrape_duration_seconds` - Scrape duration
- `github_exporter_scrape_errors_total` - Scrape error count

## License

This project is licensed under the MIT License.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests and linting
5. Submit a pull request

## Support

For issues and questions, please open an issue on GitHub.
