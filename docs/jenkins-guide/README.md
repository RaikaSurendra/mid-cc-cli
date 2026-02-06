# Jenkins CI/CD Guide for Claude Terminal MID Service

A comprehensive, beginner-friendly guide to setting up Jenkins CI/CD for the Claude Terminal MID Service project. Every example uses this project's actual codebase -- no generic "hello world" examples.

## Who This Guide Is For

- **New to Jenkins?** Start at Chapter 1 and follow the Learning Path in order.
- **Know Jenkins, new to this project?** Start at Chapter 2 (pipeline design) and Chapter 3 (project build overview).
- **Setting up Jenkins now?** Jump to Chapter 10 (setup) and Chapter 11 (Jenkinsfile walkthrough).
- **Need a quick reference?** See the Quick Reference section below.

---

## Learning Path

### Part 1: Foundation

Build a mental model of CI/CD and Jenkins before touching any configuration.

| # | Chapter | What You Will Learn |
|---|---------|-------------------|
| 1 | [What is Jenkins and Why](01-what-is-jenkins.md) | CI/CD concepts, Jenkins architecture (controller, agents, executors), how a build works end-to-end, why Jenkins for this project, terminology glossary |
| 2 | [CI/CD Pipeline Design](02-cicd-pipeline-design.md) | The 13-stage pipeline design, stage-by-stage breakdown, parallel stages, fail-fast strategy, quality gates, environment strategy, artifact flow |

### Part 2: Project Understanding

Understand what the pipeline needs to build, test, and deploy.

| # | Chapter | What You Will Learn |
|---|---------|-------------------|
| 3 | [Project Build Overview](03-project-build-overview.md) | Go module system, two-binary build process, cross-compilation, build flags, Makefile targets, dependency management |
| 4 | [Code Quality Gates](04-code-quality-gates.md) | golangci-lint configuration, gofmt enforcement, go vet checks, how each tool catches different issues |
| 5 | [Testing Strategy](05-testing-strategy.md) | Unit tests, integration tests with PostgreSQL, race detection, coverage reporting, test organization |
| 6 | [Docker Build & Push](06-docker-build-and-push.md) | Multi-stage Dockerfile walkthrough, image tagging, registry push, layer caching, Claude CLI installation |

### Part 3: Pipeline Implementation

Build and configure the actual Jenkins pipeline.

| # | Chapter | What You Will Learn |
|---|---------|-------------------|
| 7 | [Pipeline Syntax Deep Dive](07-pipeline-syntax-deep-dive.md) | Declarative vs Scripted syntax, every directive (agent, environment, stages, post, when), shared libraries, matrix builds, stash/unstash |
| 8 | [Advanced Patterns](08-advanced-jenkins-patterns.md) | Multibranch pipelines, PR builds, branch protection, quality gates, notifications, caching strategies, parallel execution, Blue Ocean |
| 9 | [Best Practices and Pitfalls](09-best-practices-and-pitfalls.md) | Top 10 mistakes beginners make, security hardening (RBAC, secrets, CSRF), performance tuning, monitoring, backup strategy |
| 10 | [Jenkins Setup](10-jenkins-setup.md) | Local Jenkins with Docker Compose, plugin installation, agent configuration, tool setup (Go, Docker, golangci-lint), first pipeline run |
| 11 | [Jenkinsfile Walkthrough](11-jenkinsfile-walkthrough.md) | Line-by-line walkthrough of the production Jenkinsfile and nightly pipeline, every decision explained |
| 12 | [Credentials & Secrets](12-credentials-and-secrets.md) | Credential types, creating and managing credentials, credentials() helper, secret masking, credential rotation |
| 13 | [Webhooks & Triggers](13-webhooks-and-triggers.md) | GitHub webhook setup, trigger types (push, PR, cron, upstream), webhook security, troubleshooting |

---

## Quick Reference

### Project Files

| File | Purpose |
|------|---------|
| `Jenkinsfile` | Main CI/CD pipeline (runs on every push) |
| `Jenkinsfile.nightly` | Nightly pipeline (full security scan, extended tests) |
| `jenkins/docker-compose.jenkins.yml` | Local Jenkins setup (controller + agent) |
| `Makefile` | Build targets used by the pipeline |
| `Dockerfile` | Multi-stage Docker build (Go build + runtime with Claude CLI) |
| `docker-compose.yml` | 4-service orchestration (postgres, http service, poller, MID server) |

### Pipeline Stages

```
Checkout --> Deps --> [Lint | Format | Vet] --> Unit Tests --> Build --> Docker Build
  --> Integration Tests --> Security Scan --> Archive --> Docker Push
  --> Deploy Staging --> Smoke Test --> [Approval] --> Deploy Prod
```

### Key Jenkins Credentials

| Credential ID | Type | Used For |
|--------------|------|----------|
| `docker-registry-credentials` | Username/Password | Docker push to registry |
| `github-token` | Secret text | GitHub API + webhook |
| `servicenow-api-credentials` | Username/Password | ServiceNow REST API |
| `anthropic-api-token` | Secret text | Claude CLI API key (integration tests) |
| `encryption-key` | Secret text | AES-256-GCM key for credential encryption |
| `postgresql-credentials` | Username/Password | Database access |

### Common Commands

```bash
# Start local Jenkins
docker compose -f jenkins/docker-compose.jenkins.yml up -d

# View Jenkins logs
docker compose -f jenkins/docker-compose.jenkins.yml logs -f jenkins

# Access Jenkins UI
open http://localhost:8080

# Run the same checks Jenkins runs, locally
make deps
golangci-lint run ./...
go vet ./...
go test -v -race -coverprofile=coverage.out ./...
make build
docker build -t claude-terminal-service:local .
```

### Pipeline Environment Variables

| Variable | Source | Example Value |
|----------|--------|---------------|
| `GIT_COMMIT` | Jenkins SCM | `abc1234def5678` |
| `BRANCH_NAME` | Jenkins SCM | `main`, `feature/websocket` |
| `BUILD_NUMBER` | Jenkins | `42` |
| `BUILD_URL` | Jenkins | `http://jenkins:8080/job/mid-llm-cli/42/` |
| `IMAGE_TAG` | Jenkinsfile | `abc1234` (first 7 chars of commit) |
| `CGO_ENABLED` | Jenkinsfile | `0` |
| `GOOS` | Jenkinsfile | `linux` |
| `GOARCH` | Jenkinsfile | `amd64` |

---

## How to Contribute to This Guide

1. Each chapter is a standalone Markdown file in `docs/jenkins-guide/`
2. Examples must use this project's actual code, files, and configuration
3. Keep the tone educational and beginner-friendly
4. When adding a new chapter, update this README's Learning Path table
5. Test all code examples (Jenkinsfile snippets, shell commands) before committing

---

## Estimated Reading Time

| Part | Chapters | Time |
|------|----------|------|
| Part 1: Foundation | 1-2 | ~30 minutes |
| Part 2: Project Understanding | 3-6 | ~40 minutes |
| Part 3: Pipeline Implementation | 7-13 | ~60 minutes |
| **Total** | **13 chapters** | **~2 hours** |
