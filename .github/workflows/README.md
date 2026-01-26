# CI/CD Workflows

**Last Updated**: 2026-01-25
**Status**: Production-ready
**Coverage**: Testing, building, releases, Docker, code quality

---

## Overview

Quidditch uses GitHub Actions for continuous integration and deployment. The CI/CD pipeline includes automated testing, code quality checks, builds for multiple platforms, Docker image creation, and automated releases.

---

## Workflows

### 1. CI Workflow (`ci.yml`)

**Triggers**: Push to `main`/`develop`, Pull Requests

**Jobs**:
- **Lint**: Run golangci-lint for code quality
- **Unit Tests**: Run on Go 1.21 and 1.22 with race detector
- **Integration Tests**: Full cluster testing with 20-minute timeout
- **Build**: Cross-compile for Linux, macOS, Windows (amd64, arm64)
- **Test Summary**: Aggregate test results

**Features**:
- Matrix builds for multiple Go versions
- Coverage reports uploaded to Codecov
- Artifacts retention (30 days)
- Integration test logs on failure

**Execution Time**: ~15-20 minutes

```yaml
# Trigger manually
gh workflow run ci.yml

# View status
gh run list --workflow=ci.yml
```

---

### 2. Release Workflow (`release.yml`)

**Triggers**: Git tags matching `v*` (e.g., `v1.0.0`)

**Jobs**:
- **Create Release**: Generate changelog and create GitHub release
- **Build and Upload**: Build binaries for all platforms and upload as release assets
- **Docker Release**: Build and push multi-arch Docker images to GHCR

**Features**:
- Automatic changelog generation from commits
- Pre-release detection (alpha, beta, rc)
- Multi-platform binaries (Linux, macOS, Windows)
- Multi-architecture Docker images (amd64, arm64)
- Version injection via ldflags

**Artifacts**:
- `quidditch-{version}-{os}-{arch}.tar.gz` (Linux/macOS)
- `quidditch-{version}-{os}-{arch}.zip` (Windows)
- Docker images: `ghcr.io/{org}/quidditch/{master|coordination|data}:version`

**Creating a Release**:
```bash
# Create and push tag
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0

# Workflow automatically:
# 1. Creates GitHub release
# 2. Builds binaries
# 3. Builds Docker images
# 4. Uploads all artifacts
```

---

### 3. Docker Build Workflow (`docker.yml`)

**Triggers**: Push to `main`/`develop` (when code changes), Manual dispatch

**Jobs**:
- **Build Master**: Build and push master node image
- **Build Coordination**: Build and push coordination node image
- **Build Data**: Build and push data node image
- **Vulnerability Scan**: Trivy security scanning

**Features**:
- Multi-architecture builds (amd64, arm64)
- Layer caching for faster builds
- Automatic tagging (branch, SHA, latest)
- Security scanning with Trivy
- Results uploaded to GitHub Security tab

**Docker Images**:
- `ghcr.io/{org}/quidditch/master:latest`
- `ghcr.io/{org}/quidditch/coordination:latest`
- `ghcr.io/{org}/quidditch/data:latest`

**Pull Images**:
```bash
# Pull latest images
docker pull ghcr.io/{org}/quidditch/master:latest
docker pull ghcr.io/{org}/quidditch/coordination:latest
docker pull ghcr.io/{org}/quidditch/data:latest

# Pull specific version
docker pull ghcr.io/{org}/quidditch/master:v1.0.0
```

---

### 4. Code Quality Workflow (`code-quality.yml`)

**Triggers**: Push, Pull Requests, Weekly schedule (Mondays)

**Jobs**:
- **Go Linting**: golangci-lint with comprehensive checks
- **Static Analysis**: staticcheck for advanced code analysis
- **Security Scan**: gosec for security vulnerabilities
- **Vulnerability Check**: govulncheck for known CVEs
- **Code Formatting**: gofmt validation
- **Go Modules**: go.mod/go.sum consistency check
- **Test Coverage**: Coverage report generation and threshold check (70%)
- **Dependency Review**: Check for vulnerable dependencies (PR only)
- **CodeQL Analysis**: Advanced security analysis

**Coverage Threshold**: 70% (enforced)

**Features**:
- Automatic security scanning
- Coverage reports as artifacts
- SARIF upload for GitHub Security
- Weekly scheduled runs for ongoing monitoring

**Run Locally**:
```bash
# Lint
golangci-lint run

# Format check
gofmt -l .

# Security scan
gosec ./...

# Vulnerability check
govulncheck ./...

# Coverage check
go test -short -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

---

## Dependabot Configuration

**File**: `.github/dependabot.yml`

**Updates**:
- **Go Modules**: Weekly on Mondays at 04:00 UTC
- **GitHub Actions**: Weekly updates
- **Docker Images**: Weekly updates

**Features**:
- Grouped dependency updates
- Automatic PR creation
- Reviewer assignment
- Commit message prefixes (deps, ci, docker)

**Limits**:
- Go modules: 10 open PRs
- GitHub Actions: 5 open PRs
- Docker: 5 open PRs

---

## Configuration Files

### golangci-lint (`.golangci.yml`)

**Enabled Linters** (25+):
- errcheck, gosimple, govet, staticcheck, unused
- gofmt, goimports, gocritic, revive
- misspell, goconst, gosec, stylecheck
- And many more...

**Settings**:
- Max complexity: 15 (gocyclo), 20 (gocognit)
- Line length: 120
- Duplication threshold: 100 lines
- Test files have relaxed rules

**Run Locally**:
```bash
golangci-lint run --timeout 5m
```

---

## Pull Request Template

**File**: `.github/pull_request_template.md`

**Sections**:
- Description and type of change
- Related issues (with auto-close support)
- Changes made
- Testing checklist (unit, integration, manual)
- Code quality checklist
- Performance impact
- Screenshots/logs
- Additional context

---

## Issue Templates

### Bug Report (`bug_report.yml`)

**Fields**:
- Bug description and expected/actual behavior
- Reproduction steps
- Component and severity
- Version and environment
- Logs and configuration
- Pre-submission checklist

### Feature Request (`feature_request.yml`)

**Fields**:
- Problem statement
- Proposed solution and alternatives
- Component and priority
- Use case and examples
- API design (if applicable)
- Compatibility considerations

---

## Secrets Required

The following secrets need to be configured in GitHub repository settings:

### Optional Secrets
- `CODECOV_TOKEN`: For Codecov integration (optional but recommended)

### Automatic Secrets (provided by GitHub)
- `GITHUB_TOKEN`: Automatically provided for GitHub API access
- Used for: Releases, GHCR push, Security scanning

---

## Status Badges

Add these to your README.md:

```markdown
[![CI](https://github.com/{org}/quidditch/workflows/CI/badge.svg)](https://github.com/{org}/quidditch/actions/workflows/ci.yml)
[![Code Quality](https://github.com/{org}/quidditch/workflows/Code%20Quality/badge.svg)](https://github.com/{org}/quidditch/actions/workflows/code-quality.yml)
[![Docker](https://github.com/{org}/quidditch/workflows/Docker%20Build/badge.svg)](https://github.com/{org}/quidditch/actions/workflows/docker.yml)
[![codecov](https://codecov.io/gh/{org}/quidditch/branch/main/graph/badge.svg)](https://codecov.io/gh/{org}/quidditch)
```

---

## Workflow Triggers Summary

| Workflow | Push | PR | Tag | Schedule | Manual |
|----------|------|----|----|----------|--------|
| CI | ✅ | ✅ | ❌ | ❌ | ❌ |
| Release | ❌ | ❌ | ✅ | ❌ | ❌ |
| Docker | ✅ | ❌ | ❌ | ❌ | ✅ |
| Code Quality | ✅ | ✅ | ❌ | ✅ | ❌ |

---

## Best Practices

### For Contributors

1. **Run Tests Locally**: Before pushing
   ```bash
   go test -short ./...
   go test ./test/integration/... -v
   ```

2. **Check Formatting**: Ensure code is formatted
   ```bash
   gofmt -w .
   ```

3. **Run Linter**: Fix linting issues
   ```bash
   golangci-lint run
   ```

4. **Use Conventional Commits**: Help with changelog generation
   ```
   feat: add new query type
   fix: resolve race condition in allocation
   docs: update API documentation
   ```

### For Maintainers

1. **Review PR Checks**: Ensure all CI checks pass before merging

2. **Semantic Versioning**: Use proper version tags
   - `v1.0.0` - Major release
   - `v1.1.0` - Minor release (new features)
   - `v1.0.1` - Patch release (bug fixes)
   - `v1.0.0-beta.1` - Pre-release

3. **Security**: Review Dependabot PRs promptly

4. **Coverage**: Monitor test coverage trends

---

## Troubleshooting

### CI Failures

**Problem**: Tests fail in CI but pass locally
**Solutions**:
- Check Go version compatibility (CI uses 1.21 and 1.22)
- Run with race detector: `go test -race ./...`
- Check for timing issues in tests
- Ensure tests are not dependent on local environment

**Problem**: Integration tests timeout
**Solutions**:
- Increase timeout in workflow (current: 20m)
- Optimize cluster startup time
- Check for resource constraints

### Docker Build Failures

**Problem**: Multi-arch build fails
**Solutions**:
- Check Dockerfile syntax
- Ensure QEMU is properly set up
- Verify base image supports target architecture

### Linting Failures

**Problem**: golangci-lint reports errors
**Solutions**:
- Run locally: `golangci-lint run`
- Check `.golangci.yml` configuration
- Fix or add `//nolint:lintername` comments if needed

---

## Performance Metrics

### Typical Workflow Times

| Workflow | Duration | Notes |
|----------|----------|-------|
| CI (unit tests) | 3-5 min | Per Go version |
| CI (integration) | 10-15 min | Full cluster |
| CI (build) | 5-10 min | Per platform |
| Docker build | 8-12 min | Per image |
| Code quality | 5-8 min | All checks |
| Release | 25-35 min | Full pipeline |

### Resource Usage

- **Concurrent jobs**: Up to 20 (GitHub default)
- **Storage**: ~500 MB per build (artifacts)
- **Bandwidth**: ~2 GB per release (all platforms)

---

## Future Enhancements

### Planned
- [ ] Performance regression testing
- [ ] Automated benchmarking
- [ ] End-to-end testing with real clusters
- [ ] Nightly builds
- [ ] Canary deployments
- [ ] Auto-merge for Dependabot PRs (with tests)

---

## Support

- **Documentation**: See workflow files for inline comments
- **Issues**: Report CI/CD issues with label `ci/cd`
- **Discussions**: Ask questions in GitHub Discussions

---

## Summary

| Component | Files | Status |
|-----------|-------|--------|
| CI Workflow | ci.yml | ✅ Complete |
| Release Workflow | release.yml | ✅ Complete |
| Docker Workflow | docker.yml | ✅ Complete |
| Code Quality | code-quality.yml | ✅ Complete |
| Dependabot | dependabot.yml | ✅ Complete |
| Linting Config | .golangci.yml | ✅ Complete |
| PR Template | pull_request_template.md | ✅ Complete |
| Issue Templates | bug_report.yml, feature_request.yml | ✅ Complete |
| **Total** | **8 files** | **✅ Complete** |

---

**Status**: ✅ Production-ready
**Maintenance**: Automated via Dependabot
**Coverage**: Full CI/CD pipeline with quality gates

---

Last updated: 2026-01-25
