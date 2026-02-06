# Chapter 2: CI/CD Pipeline Design for This Project

## The Pipeline at a Glance

Every push to the Claude Terminal MID Service repository triggers the following automated pipeline:

```
+----------+     +------+     +------------------+     +------------+     +-------+
| Checkout |---->| Deps |---->| Quality Checks   |---->| Unit Tests |---->| Build |
+----------+     +------+     | (parallel)       |     +------------+     +-------+
                               | +------+         |                           |
                               | | Lint |         |                           v
                               | +------+         |                     +-----------+
                               | +--------+       |                     | Docker    |
                               | | Format |       |                     | Build     |
                               | +--------+       |                     +-----------+
                               | +-----+          |                           |
                               | | Vet |          |                           v
                               | +-----+          |                     +-----------+
                               +------------------+                     | Integ.    |
                                                                        | Tests     |
                                                                        +-----------+
                                                                              |
                +------+     +---------+     +-------+     +---------+        v
                | Prod |<----| Staging |<----| Push  |<----| Archive |<--+-----------+
                +------+     +---------+     +-------+     +---------+   | Security |
                   ^              ^                                       | Scan     |
                   |              |                                       +-----------+
               [approval]    [smoke test]
```

### Complete Pipeline Flow

```
Checkout --> Deps --> [Lint | Format | Vet] --> Unit Tests --> Build --> Docker Build
  --> Integration Tests (postgres + service) --> Security Scan --> Archive
  --> Docker Push --> Deploy Staging --> Smoke Test --> Deploy Prod
```

---

## Stage-by-Stage Breakdown

### Stage 1: Checkout

**What it does:** Clones the Git repository and checks out the specific commit that triggered the build.

**Why it is here:** Everything starts with the source code. Jenkins needs a clean copy of the code at the exact commit being built.

**What it looks like in Jenkins:**
```groovy
stage('Checkout') {
    steps {
        checkout scm   // Uses the SCM configured in the pipeline
    }
}
```

**What failure looks like:**
- `ERROR: Error cloning remote repo 'origin'` -- Git credentials are wrong or the repo is unreachable
- `ERROR: Couldn't find any revision to build` -- The branch was deleted between trigger and checkout

**What moves to the next stage:** The entire source tree in the workspace directory.

---

### Stage 2: Dependencies

**What it does:** Downloads Go module dependencies and ensures `go.mod` and `go.sum` are in sync.

**Why it is here:** All subsequent stages (lint, test, build) need dependencies resolved. Doing this once avoids redundant downloads.

**The commands:**
```bash
go mod download    # Download all modules to local cache
go mod tidy        # Ensure go.mod and go.sum are consistent
```

**What failure looks like:**
- `go: module example.com/some/pkg: no matching versions` -- A dependency was removed or renamed
- `go.sum mismatch` -- Someone modified `go.sum` incorrectly, or a dependency was tampered with
- Network timeout -- The agent cannot reach `proxy.golang.org`

**What moves to the next stage:** A populated Go module cache (`$GOPATH/pkg/mod/`).

---

### Stage 3: Quality Checks (Parallel)

This stage runs three checks **simultaneously**. They are independent of each other, so running them in parallel saves time.

```groovy
stage('Quality Checks') {
    parallel {
        stage('Lint')   { steps { sh 'golangci-lint run ./...' } }
        stage('Format') { steps { sh 'test -z "$(gofmt -l .)"' } }
        stage('Vet')    { steps { sh 'go vet ./...' } }
    }
}
```

#### Why Parallel?

Consider the time savings:

```
Sequential:                     Parallel:
  Lint    [====60s====]           Lint    [====60s====]
  Format  [==20s==]               Format  [==20s==]        } All 3 run at once
  Vet     [===30s===]             Vet     [===30s===]
  Total:  110 seconds             Total:  60 seconds (max of the three)
```

These three checks examine the code but do not modify it, and they do not depend on each other's output. That makes them safe to parallelize.

#### Stage 3a: Lint

**What it does:** Runs `golangci-lint`, a meta-linter that aggregates 50+ Go linting tools (staticcheck, gosec, errcheck, ineffassign, and more).

**Why it is here:** Catches bugs, security issues, and style violations before tests run. Cheaper to fix a lint error than debug a runtime failure.

**What failure looks like:**
```
internal/session/session.go:142:6: Error return value of `conn.Close` is not checked (errcheck)
internal/crypto/crypto.go:87:2: G404: Use of weak random number generator (gosec)
```

**Fail-fast behavior:** If lint fails, the entire pipeline stops. There is no point building and testing code that has known issues.

#### Stage 3b: Format

**What it does:** Checks that all Go files are formatted according to `gofmt` standards.

**Why it is here:** Consistent formatting eliminates style debates in code reviews. If `gofmt -l .` produces any output, files are not formatted correctly.

**What failure looks like:**
```
internal/server/server.go
internal/middleware/ratelimit.go
FAIL: 2 files not formatted. Run 'go fmt ./...'
```

#### Stage 3c: Vet

**What it does:** Runs `go vet`, the built-in Go static analysis tool. It detects issues the compiler does not catch: unreachable code, incorrect format strings, struct tag errors.

**Why it is here:** `go vet` catches subtle bugs that pass compilation but fail at runtime.

**What failure looks like:**
```
./internal/session/session.go:201:2: Printf call has arguments but no formatting directives
```

---

### Stage 4: Unit Tests

**What it does:** Runs the full test suite with the race detector and generates a coverage report.

**The commands:**
```bash
go test -v -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

**Why it is here:** Tests verify that the code behaves correctly. The race detector catches data races in the concurrent session manager (critical for a PTY-based service). Coverage tracking ensures tests are comprehensive.

**What failure looks like:**
```
--- FAIL: TestSessionManager_CreateSession (0.03s)
    session_test.go:45: expected session status "active", got "terminated"
FAIL    github.com/user/mid-llm-cli/internal/session   0.234s
```

**Key detail for this project:** The session manager uses goroutines extensively (PTY I/O, output buffering, timeout monitoring). The `-race` flag is essential to catch concurrent access bugs that only surface under load.

**What moves to the next stage:** `coverage.out` (for archival) and a green test status.

---

### Stage 5: Build

**What it does:** Compiles the two Go binaries for Linux (the target platform for Docker and production).

**The commands:**
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/claude-terminal-service ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/ecc-poller ./cmd/ecc-poller
```

**Why it is here:** This produces the actual deliverable — the binaries that run in production.

**Flag explanations:**
| Flag | Purpose |
|------|---------|
| `CGO_ENABLED=0` | Pure Go binary. No C dependencies. Ensures the binary runs on Alpine Linux (musl libc). |
| `GOOS=linux` | Cross-compile for Linux regardless of the build agent's OS. |
| `GOARCH=amd64` | Target x86_64 architecture. |
| `-ldflags "-s -w"` | Strip debug symbols and DWARF info. Reduces binary size by ~30%. |

**What failure looks like:**
```
./cmd/server/main.go:15:2: cannot find package "github.com/gin-gonic/gin"
```
This should not happen if the Deps stage passed, but a corrupted module cache could cause it.

**What moves to the next stage:** Two static binaries in `bin/`.

---

### Stage 6: Docker Build

**What it does:** Builds the production Docker image using the multi-stage Dockerfile.

**The command:**
```bash
docker build -t claude-terminal-service:${BUILD_TAG} .
```

**Why it is here:** The Docker image is the deployment artifact. It packages the Go binary, Node.js (for Claude CLI), and all runtime dependencies into a single portable unit.

**What the Dockerfile does (recap from the project):**
1. **Stage 1 (builder):** Uses `golang:1.24-alpine` to compile the binaries
2. **Stage 2 (runtime):** Uses `alpine:latest`, installs Node.js + Claude CLI, copies binaries from stage 1

**What failure looks like:**
```
Step 7/15 : RUN npm install -g @anthropic-ai/claude-code
ERROR: npm ERR! network timeout
```
Docker builds can fail for network reasons (downloading base images, installing npm packages) or for Dockerfile syntax errors.

**What moves to the next stage:** A tagged Docker image in the local Docker daemon.

---

### Stage 7: Integration Tests

**What it does:** Spins up PostgreSQL and the Claude Terminal Service in Docker, then runs integration tests against the live system.

**Why it is here:** Unit tests mock external dependencies. Integration tests verify that the service actually talks to PostgreSQL, handles HTTP requests correctly, and manages sessions end-to-end.

**The setup:**
```bash
# Start test dependencies
docker compose -f docker-compose.test.yml up -d postgres
# Wait for PostgreSQL to be ready
until docker exec test-postgres pg_isready; do sleep 1; done
# Run integration tests
go test -v -tags=integration ./...
# Tear down
docker compose -f docker-compose.test.yml down -v
```

**What gets tested:**
- PostgreSQL session persistence (create, read, update, delete)
- HTTP endpoint responses (health check, session CRUD)
- Authentication and authorization (bearer token, user ID validation)
- Rate limiting behavior under load
- Encryption/decryption round-trip for credentials

**What failure looks like:**
```
--- FAIL: TestIntegration_SessionPersistence (2.15s)
    store_test.go:89: expected 1 session in database, found 0
```

**Important:** This stage requires Docker-in-Docker or a Docker socket mount on the Jenkins agent.

---

### Stage 8: Security Scan

**What it does:** Scans the codebase and Docker image for known vulnerabilities.

**The commands:**
```bash
# Go dependency vulnerabilities
govulncheck ./...

# Docker image vulnerabilities
trivy image claude-terminal-service:${BUILD_TAG}
```

**Why it is here:** Dependencies can have known CVEs. The Docker base image (`alpine:latest`) may ship with vulnerable packages. Catching these in CI prevents deploying vulnerable software.

**What failure looks like:**
```
govulncheck:
  Vulnerability #1: GO-2024-2887
    A malicious HTTP/2 client sending rapidly reset streams
    Found in: golang.org/x/net@v0.17.0
    Fixed in: golang.org/x/net@v0.23.0

trivy:
  CRITICAL: CVE-2024-12345 in libcurl 8.5.0 (fixed in 8.6.0)
```

**Policy decisions:**
- **Critical/High CVEs:** Fail the build. Block deployment.
- **Medium/Low CVEs:** Warn but allow the build to proceed.

---

### Stage 9: Archive Artifacts

**What it does:** Saves build outputs for later use and traceability.

**Archived artifacts:**
| Artifact | Purpose |
|----------|---------|
| `bin/claude-terminal-service` | The server binary |
| `bin/ecc-poller` | The poller binary |
| `coverage.out` | Test coverage data |
| `coverage.html` | Human-readable coverage report |
| `trivy-report.json` | Security scan results |
| Docker image digest | Immutable reference to the built image |

**Why it is here:** Artifacts provide an audit trail. If production breaks, you can download the exact binary that was deployed and compare it with a working version.

---

### Stage 10: Docker Push

**What it does:** Pushes the Docker image to a container registry.

**The command:**
```bash
docker push registry.example.com/mid-llm-cli/claude-terminal-service:${BUILD_TAG}
docker push registry.example.com/mid-llm-cli/claude-terminal-service:latest
```

**Why it is here:** The registry is the distribution point. Kubernetes, Docker Compose, and manual deployments all pull from the registry.

**Tagging strategy:**
```
registry.example.com/mid-llm-cli/claude-terminal-service:1.2.3        # Semantic version
registry.example.com/mid-llm-cli/claude-terminal-service:abc1234      # Git commit SHA
registry.example.com/mid-llm-cli/claude-terminal-service:latest       # Latest build from main
```

---

### Stage 11: Deploy to Staging

**What it does:** Deploys the new image to the staging environment.

**Why it is here:** Staging is a production-like environment where the service is tested with real (or realistic) ServiceNow connections before reaching production.

**The deployment method depends on infrastructure:**
```bash
# Kubernetes
kubectl set image deployment/claude-terminal \
  claude-terminal=registry.example.com/mid-llm-cli/claude-terminal-service:${BUILD_TAG} \
  -n staging

# Docker Compose (on a staging server)
ssh staging "cd /opt/claude-terminal && docker compose pull && docker compose up -d"
```

---

### Stage 12: Smoke Test

**What it does:** Runs a minimal set of tests against the staging deployment to verify it is working.

**The checks:**
```bash
# Health endpoint responds
curl -f https://staging.example.com/health

# Service returns valid JSON
curl -s https://staging.example.com/health | jq .status

# Session creation works (with test credentials)
curl -X POST https://staging.example.com/api/session/create \
  -H "Authorization: Bearer ${STAGING_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"userId":"smoke-test","workspaceType":"isolated"}'
```

**Why it is here:** The Docker image may build and pass tests locally, but the staging environment could have different networking, DNS, or ServiceNow connectivity. Smoke tests catch deployment-specific failures.

---

### Stage 13: Deploy to Production

**What it does:** Deploys the verified image to the production environment.

**Why it is last:** Production is the highest-risk environment. Every preceding stage is a gate that must pass before code reaches users.

**Approval gate:**
```groovy
stage('Deploy Production') {
    input {
        message "Deploy to production?"
        ok "Yes, deploy"
        submitter "admin,deploy-team"
    }
    steps {
        // Same deployment commands as staging, targeting production
    }
}
```

The `input` directive pauses the pipeline and waits for a human to approve the deployment. Only users in the `admin` or `deploy-team` groups can approve.

---

## Pipeline Design Principles

### Fail-Fast Strategy

The pipeline is ordered so that cheap, fast checks run first:

```
Lint (60s) --> Tests (90s) --> Build (30s) --> Docker (120s) --> Integration (180s) --> Deploy
  ^                                                                                      ^
  |                                                                                      |
  Cheapest / fastest                                                           Most expensive
  Fail here = save 7+ minutes                                                 Fail here = rare
```

If formatting is wrong, there is no reason to spend 3 minutes building a Docker image. Fail early, fail fast, fix quickly.

### Parallel Where Safe

Stages can run in parallel when:
1. They do not depend on each other's output
2. They do not modify shared state
3. They read the same input (source code)

**Safe to parallelize:** Lint, Format, Vet (all read source, none modify it)
**Not safe to parallelize:** Build depends on Deps, Docker Build depends on Build

### Stage Dependencies

```
Checkout
  |
  v
Deps
  |
  v
[Lint | Format | Vet]  <-- parallel, all depend on Deps
  |
  v
Unit Tests              <-- depends on Deps (for test dependencies)
  |
  v
Build                   <-- depends on Deps (for compilation)
  |
  v
Docker Build            <-- depends on Build (copies binaries into image)
  |
  v
Integration Tests       <-- depends on Docker Build (tests the container)
  |
  v
Security Scan           <-- depends on Docker Build (scans the image)
  |
  v
Archive + Push          <-- depends on all prior stages passing
  |
  v
Deploy Staging          <-- depends on Push (image must be in registry)
  |
  v
Smoke Test              <-- depends on Deploy Staging (service must be running)
  |
  v
Deploy Production       <-- depends on Smoke Test + human approval
```

---

## Environment Strategy

| Environment | Purpose | Trigger | Approval |
|------------|---------|---------|----------|
| **CI** | Build, test, scan | Every push, every PR | None (automatic) |
| **Staging** | Production-like validation | Merge to `main` | None (automatic) |
| **Production** | Live user-facing service | After staging smoke test | Manual approval required |

### Branch Strategy

| Branch | Pipeline Behavior |
|--------|------------------|
| `feature/*` | Run CI stages only (lint, test, build). Do not deploy. |
| `develop` | Run CI stages + Docker build + push to dev registry. |
| `main` | Run full pipeline including staging and production deployment. |
| Pull Requests | Run CI stages. Post status check to GitHub. Block merge if failing. |

---

## Artifact Flow

Here is what moves between stages:

```
[Deps]
  |
  +-- Go module cache ($GOPATH/pkg/mod/)
  |
  v
[Build]
  |
  +-- bin/claude-terminal-service (Linux amd64 binary, ~15MB)
  +-- bin/ecc-poller (Linux amd64 binary, ~12MB)
  |
  v
[Docker Build]
  |
  +-- claude-terminal-service:abc1234 (Docker image, ~250MB)
  |
  v
[Unit Tests]
  |
  +-- coverage.out (coverage data)
  +-- coverage.html (HTML report)
  +-- test-results.xml (JUnit format for Jenkins parsing)
  |
  v
[Security Scan]
  |
  +-- trivy-report.json (image scan results)
  +-- govulncheck-report.txt (Go dependency scan)
  |
  v
[Archive]
  |
  +-- All of the above, stored in Jenkins build artifacts
```

---

## Quality Gates: "You Shall Not Pass"

A **quality gate** is a checkpoint that blocks the pipeline if quality criteria are not met. Think of it as a toll booth on a highway — you cannot proceed without paying the toll.

### Gates in This Pipeline

| Gate | Criteria | Blocks |
|------|----------|--------|
| **Lint Gate** | Zero lint errors from golangci-lint | Building, testing, deploying |
| **Format Gate** | All files pass `gofmt` | Building, testing, deploying |
| **Test Gate** | All tests pass, no race conditions | Building Docker image |
| **Coverage Gate** | Coverage >= 70% (configurable threshold) | Docker build |
| **Security Gate** | No Critical/High CVEs | Pushing to registry, deploying |
| **Smoke Test Gate** | Health endpoint returns 200 OK | Production deployment |
| **Approval Gate** | Human approves production deploy | Production deployment |

### How Gates Work in the Jenkinsfile

```groovy
stage('Coverage Check') {
    steps {
        script {
            def coverage = sh(
                script: "go tool cover -func=coverage.out | grep total | awk '{print \$3}' | sed 's/%//'",
                returnStdout: true
            ).trim().toFloat()

            if (coverage < 70.0) {
                error("Coverage ${coverage}% is below the 70% threshold. Build blocked.")
            }
            echo "Coverage: ${coverage}% -- GATE PASSED"
        }
    }
}
```

The `error()` function immediately fails the stage, which stops the pipeline. No Docker image gets built, no deployment happens.

---

## Project-Specific Design Decisions

### Two Binaries, One Pipeline

The project produces two binaries from the same repository:
- `claude-terminal-service` (HTTP server)
- `ecc-poller` (ServiceNow ECC Queue client)

Both are built, tested, and packaged in the same pipeline. They share the same Go modules, the same test suite, and the same Docker image (the poller runs the same image with a different command: `./ecc-poller`).

### Claude CLI in the Docker Image

The Dockerfile installs Node.js and `@anthropic-ai/claude-code` in the runtime image. This means:
- The Docker build stage requires network access to npm
- The npm install step can be slow (~30-60 seconds)
- The npm install can fail if the npm registry is unreachable
- The image is larger than a pure Go binary image (~250MB vs ~20MB)

The pipeline should cache the npm install layer aggressively using Docker BuildKit cache mounts.

### PostgreSQL for Integration Tests

Integration tests need a running PostgreSQL 15 instance. The pipeline:
1. Starts PostgreSQL via Docker Compose
2. Waits for the health check (`pg_isready`)
3. Runs integration tests against it
4. Tears down the container and deletes the volume

This requires the Jenkins agent to have Docker access (Docker socket or Docker-in-Docker).

### ServiceNow Connectivity

Full end-to-end tests would require a ServiceNow instance. The pipeline handles this in two tiers:
- **CI tests:** Mock ServiceNow API responses (no instance needed)
- **Staging smoke tests:** Test against a real ServiceNow dev instance

---

## Pipeline Timing Estimate

For a typical commit on an agent with 4 CPUs and 8GB RAM:

| Stage | Duration | Cumulative |
|-------|----------|------------|
| Checkout | ~5s | 5s |
| Dependencies | ~15s (cached) / ~45s (cold) | 20s |
| Quality Checks (parallel) | ~60s | 1m 20s |
| Unit Tests | ~90s | 2m 50s |
| Build | ~30s | 3m 20s |
| Docker Build | ~120s (cached) / ~300s (cold) | 5m 20s |
| Integration Tests | ~180s | 8m 20s |
| Security Scan | ~60s | 9m 20s |
| Archive + Push | ~30s | 9m 50s |
| Deploy Staging | ~60s | 10m 50s |
| Smoke Test | ~15s | 11m 05s |
| **Total (cached)** | | **~11 minutes** |

The goal: a developer pushes code and gets feedback within 12 minutes. With good caching, most CI-only runs (no deployment) complete in under 5 minutes.

---

## Summary

The pipeline for the Claude Terminal MID Service moves code through 13 stages, from checkout to production deployment. Quality gates at every stage ensure that only well-formatted, tested, secure code reaches production. Parallel stages save time, fail-fast ordering minimizes wasted compute, and human approval gates protect the production environment.

---

**Previous:** [Chapter 1: What is Jenkins and Why](01-what-is-jenkins.md)
**Next:** [Chapter 3: Project Build Overview](03-project-build-overview.md)
