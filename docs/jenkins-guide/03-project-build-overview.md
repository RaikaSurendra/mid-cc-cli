# Chapter 3: Understanding the Project Build

## What This Project Is

The Claude Terminal MID Service is a Go-based backend that bridges Anthropic's Claude Code CLI with ServiceNow via MID Servers. It exposes a REST API (built on the Gin HTTP framework) that manages interactive pseudo-terminal (PTY) sessions. When a ServiceNow user starts a Claude session from the ServiceNow portal, this service spawns a real Claude CLI process, pipes input/output through a PTY, and persists session state in PostgreSQL.

The project produces two separate binaries from a single codebase. The first, `claude-terminal-service`, is the HTTP server that handles session lifecycle (create, command, output, resize, terminate) and health checks. The second, `ecc-poller`, continuously polls the ServiceNow ECC Queue for inbound commands and translates them into HTTP calls against the first service. Together they form the Go backend that sits between ServiceNow's MID Server infrastructure and the Claude Code CLI running on the host.

---

## Build Toolchain

This project uses the following tools to compile and package:

| Tool | Version | Purpose |
|------|---------|---------|
| **Go** | 1.24+ | Compiler and standard toolchain |
| **GNU Make** | any | Build orchestration (targets for build, test, lint, etc.) |
| **Docker** | 20.10+ | Multi-stage container builds for deployment |
| **Alpine Linux** | latest | Minimal base image for the runtime container |

The Go version is pinned in `go.mod`:

```
go 1.24.0
toolchain go1.24.1
```

Jenkins must have Go 1.24+ installed (or use a Docker-based build agent) to compile this project. The `go.mod` file ensures that the exact toolchain version is used regardless of what is installed on the agent, as long as the minimum version requirement is met.

---

## Two Binaries, One Codebase

The project compiles into two distinct executables. Both live under the `cmd/` directory, which is the standard Go project layout for multiple entry points:

```
cmd/
  server/main.go       --> bin/claude-terminal-service
  ecc-poller/main.go   --> bin/ecc-poller
```

**claude-terminal-service** (`cmd/server/main.go`):
- HTTP REST API server on port 3000
- Manages PTY sessions (create, command, output, resize, terminate)
- Health check endpoint at `/health`
- Bearer token authentication, CORS, rate limiting
- Optional PostgreSQL-backed session persistence
- Graceful shutdown with signal handling (SIGINT, SIGTERM)

**ecc-poller** (`cmd/ecc-poller/main.go`):
- Polls the ServiceNow ECC Queue every 5 seconds
- Translates ECC Queue items into HTTP calls to the terminal service
- Processes up to 5 items concurrently (worker pool pattern)
- Per-item 30-second context timeout
- Updates ECC Queue items with results or errors

---

## Build Commands

The `Makefile` provides the following build targets:

### Build Everything

```bash
make build
```

This runs both `build-server` and `build-poller` sequentially. It is equivalent to:

```bash
make build-server
make build-poller
```

### Build Individual Binaries

```bash
# Build only the HTTP service
make build-server

# Build only the ECC Queue poller
make build-poller
```

Each target:
1. Creates the `bin/` directory if it does not exist (`mkdir -p bin`)
2. Runs `go build` with linker flags targeting the appropriate `cmd/` package
3. Outputs the binary to `bin/<name>`

### Full Rebuild (Clean + Dependencies + Build)

```bash
make all
```

This runs `clean`, then `deps`, then `build` -- a complete from-scratch build. This is typically what Jenkins should run to ensure a clean, reproducible build.

---

## Build Flags: `-ldflags "-s -w"`

The Makefile uses these linker flags for every build:

```makefile
LDFLAGS=-ldflags "-s -w"
```

Here is what each flag does:

| Flag | Effect | Why |
|------|--------|-----|
| `-s` | Strip the symbol table | Removes debugging symbols. Reduces binary size by ~25%. Prevents reverse-engineering of internal function names. |
| `-w` | Strip DWARF debug information | Removes debug data used by tools like `gdb` and `delve`. Further reduces binary size. |

**Combined effect**: A Go binary that might be 25 MB unstripped becomes roughly 17-18 MB with these flags. For production Docker images, smaller binaries mean faster image pulls and less disk usage.

**Trade-off**: You cannot attach a debugger (e.g., Delve) to a stripped binary. For development and debugging, you would build without these flags:

```bash
go build -o bin/claude-terminal-service ./cmd/server
```

In Jenkins, always use the stripped flags for production builds. If you need a debug build for troubleshooting, create a separate Makefile target or Jenkins parameter.

---

## Dependencies

### go.mod Analysis

The project's `go.mod` declares 7 direct dependencies:

| Dependency | Version | Purpose |
|-----------|---------|---------|
| `github.com/creack/pty` | v1.1.24 | Pseudo-terminal management for spawning Claude CLI |
| `github.com/gin-gonic/gin` | v1.11.0 | HTTP framework for the REST API |
| `github.com/google/uuid` | v1.6.0 | UUID generation for session IDs |
| `github.com/jackc/pgx/v5` | v5.8.0 | PostgreSQL driver for session persistence |
| `github.com/joho/godotenv` | v1.5.1 | `.env` file loading for configuration |
| `github.com/sirupsen/logrus` | v1.9.4 | Structured logging |
| `golang.org/x/time` | v0.14.0 | Rate limiter (`rate.Limiter`) for per-IP throttling |

There are also ~25 indirect (transitive) dependencies pulled in by these libraries. The full dependency tree is locked in `go.sum`.

### Downloading Dependencies

```bash
# Download all dependencies to the local module cache
go mod download

# Or use the Makefile target (which also runs tidy):
make deps
```

`go mod download` fetches every module listed in `go.sum` into `$GOPATH/pkg/mod/`. In Jenkins, this step should run **before** compilation to ensure all dependencies are available. It also makes build failures easier to diagnose -- a dependency download failure is distinct from a compilation failure.

### Tidying Dependencies

```bash
go mod tidy
```

This removes unused dependencies and adds any missing ones. The `make deps` target runs both download and tidy:

```makefile
deps:
    @$(GOMOD) download
    @$(GOMOD) tidy
```

In a CI pipeline, you can also run `go mod tidy` and then check if `go.mod` or `go.sum` changed. If they did, someone forgot to commit dependency updates:

```bash
go mod tidy
git diff --exit-code go.mod go.sum
```

This is a useful Jenkins quality gate to ensure dependency files are always committed in a consistent state.

---

## Output Artifacts

After a successful `make build`, the `bin/` directory contains:

```
bin/
  claude-terminal-service    # HTTP API server (~18 MB stripped)
  ecc-poller                 # ECC Queue polling service (~15 MB stripped)
```

Additional artifacts produced by other Makefile targets:

| File | Produced By | Description |
|------|------------|-------------|
| `coverage.out` | `make test-coverage` | Go coverage profile (machine-readable) |
| `coverage.html` | `make test-coverage` | Coverage report (human-readable HTML) |
| `benchmarks/*.txt` | `make bench-full` | Benchmark result files |
| `benchmarks/*.prof` | `make bench-full` | CPU and memory profiles |

In Jenkins, you would archive `bin/*` as build artifacts and `coverage.html` as a test report artifact.

---

## How This Maps to a Jenkins Stage

Here is how the build process translates to a Jenkins pipeline stage. This is what Jenkins would actually execute:

```groovy
stage('Build') {
    steps {
        // Step 1: Download and verify dependencies
        sh 'go mod download'
        sh 'go mod tidy'

        // Step 2: Verify no uncommitted dependency changes
        sh 'git diff --exit-code go.mod go.sum'

        // Step 3: Compile both binaries with stripped flags
        sh 'make build'

        // Step 4: Verify binaries were created
        sh 'ls -la bin/claude-terminal-service bin/ecc-poller'

        // Step 5: Archive the binaries as Jenkins artifacts
        archiveArtifacts artifacts: 'bin/*', fingerprint: true
    }
}
```

**What happens at each step:**

1. **`go mod download`** -- Fetches all dependencies into the Go module cache. This is separated from compilation so that dependency failures are clearly distinguishable from code failures.

2. **`go mod tidy`** -- Ensures the dependency graph is consistent. This catches situations where a developer added a new import but forgot to run `go mod tidy` before pushing.

3. **`git diff --exit-code go.mod go.sum`** -- A quality gate. If `go mod tidy` changed either file, the build fails. This forces developers to keep their dependency files in sync.

4. **`make build`** -- Compiles both `claude-terminal-service` and `ecc-poller` into the `bin/` directory with production-optimized linker flags (`-s -w`).

5. **`ls -la bin/*`** -- A simple verification that both binaries exist and have reasonable file sizes. Catches edge cases where `go build` succeeds but produces a zero-byte output.

6. **`archiveArtifacts`** -- Jenkins archives the compiled binaries so they can be downloaded later, used in subsequent pipeline stages (Docker build, deployment), or attached to release tags.

### Environment Requirements for the Jenkins Agent

For this stage to succeed, the Jenkins agent needs:

- **Go 1.24+** installed and on `PATH` (or use the Jenkins Go plugin / Docker agent)
- **GNU Make** installed (`apt-get install make` on Debian/Ubuntu)
- **Git** installed (for the `git diff` check)
- **Network access** to `proxy.golang.org` (or a configured `GOPROXY`) for downloading modules

A common pattern is to use a Docker-based Jenkins agent with Go pre-installed:

```groovy
pipeline {
    agent {
        docker {
            image 'golang:1.24-alpine'
            args '-v go-mod-cache:/go/pkg/mod'  // Persist module cache between builds
        }
    }
    // ... stages ...
}
```

The `-v go-mod-cache:/go/pkg/mod` argument mounts a Docker volume for the Go module cache, so subsequent builds do not need to re-download all dependencies from scratch. This can reduce build times from minutes to seconds.
