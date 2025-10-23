# GitHub Exporter

A Prometheus exporter for GitHub metrics that collects repository statistics, organization data, and user information from the GitHub API.

## Features

- **Repository Metrics**: Stars, forks, issues, pull requests, and more
- **Organization Monitoring**: Track multiple organizations and their repositories
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

## Rate Limiting

The exporter automatically manages GitHub API rate limits:

- Fetches current rate limit status from GitHub API
- Calculates optimal refresh intervals
- Respects rate limit buffers to avoid hitting limits
- Provides rate limit metrics for monitoring

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
