# Chapter 5: Testing Strategy for CI

## How Go Tests Work

Go has a built-in testing framework -- there is no need to install a third-party library like JUnit (Java) or pytest (Python). Any file named `*_test.go` is automatically recognized as a test file. Functions in these files whose names start with `Test` are unit tests, and functions starting with `Benchmark` are performance benchmarks.

To run all tests in this project:

```bash
go test -v ./...
```

The `./...` pattern means "this package and all sub-packages." The `-v` flag enables verbose output, printing each test name and its pass/fail status.

---

## Where Tests Live

This project has three test files, each testing a different internal package:

```
internal/config/config_test.go       (4 tests, 1 benchmark)
internal/session/session_test.go     (10 tests, 5 benchmarks)
internal/server/server_test.go       (9 tests, 3 benchmarks)
```

---

## Test File: `internal/config/config_test.go`

**What it validates:** Configuration loading from environment variables.

| Test Function | What It Checks |
|--------------|----------------|
| `TestLoadConfig` | Sets `SERVICENOW_INSTANCE`, `SERVICENOW_API_USER`, and `SERVICENOW_API_PASSWORD` via `t.Setenv()`, then verifies `config.Load()` returns the correct values. Confirms the config system reads environment variables correctly. |
| `TestLoadConfigMissingRequired` | Calls `config.Load()` without setting required environment variables. Verifies it returns an error rather than silently using empty strings. This protects against deploying with missing configuration. |
| `TestDefaultValues` | Sets only the required variables and verifies default values: port 3000, session timeout 30 minutes, max 3 sessions per user, workspace type "isolated". Ensures sensible defaults when optional config is omitted. |
| `TestCustomValues` | Sets custom values for optional config (port 8080, timeout 60m, max 5 sessions, workspace "persistent"). Verifies overrides take effect. |

**Benchmark:**

| Benchmark Function | What It Measures |
|-------------------|-----------------|
| `BenchmarkLoadConfig` | How fast `config.Load()` runs. Establishes a performance baseline for configuration parsing. |

**Key pattern:** Tests use `t.Setenv()` which automatically restores the original environment variable value after the test completes. This prevents test pollution -- one test's environment changes cannot affect another test.

---

## Test File: `internal/session/session_test.go`

**What it validates:** The PTY session manager -- the core of the application.

| Test Function | What It Checks |
|--------------|----------------|
| `TestNewManager` | Creates a new session manager and verifies it initializes with a non-nil sessions map containing zero entries. Basic constructor validation. |
| `TestSessionCreation` | Attempts to create a real session with a test API key. If Claude CLI is not installed (which is the case in CI), the test gracefully logs the error instead of failing. This is a resilient test design -- it validates the code path without requiring external dependencies. |
| `TestInvalidUserIDRejected` | Sends path traversal attacks (`../../../etc`) and null byte injection (`user\x00id`) as user IDs. Verifies both are rejected with errors. This is a **security test** that ensures the session manager blocks directory traversal via crafted user IDs. |
| `TestSessionLimit` | Manually adds 2 sessions for a user (matching the configured limit of 2), then attempts to create a third. Verifies the limit is enforced with an error. Prevents resource exhaustion by a single user. |
| `TestSessionTimeout` | Creates two sessions: one with `LastActivity` 2 minutes ago (past the 1-minute timeout) and one that is current. Runs `checkTimeouts()` and verifies the old session is removed while the recent one survives. |
| `TestOutputBuffer` | Adds 3 output chunks to a session buffer. Verifies `GetOutput(false)` returns all 3 without clearing, and `GetOutput(true)` returns all 3 and clears the buffer to zero. |
| `TestOutputBufferLimit` | Adds 150 output chunks to a buffer with a limit of 100. Verifies the buffer never exceeds 100 entries. Prevents unbounded memory growth. |
| `TestOutputBufferCustomSize` | Same as above but with a custom buffer size of 50. Verifies the configurable limit is respected. |
| `TestSessionStatus` | Creates a session and calls `GetStatus()`. Verifies the returned map contains the correct session_id, user_id, status, and output_buffer_size. |
| `TestSanitizeCommand` | Table-driven test with 8 cases testing the command sanitizer. Verifies that printable characters, newlines (`\n`), carriage returns (`\r`), and tabs (`\t`) are preserved, while null bytes (`\x00`), escape sequences (`\x1b`), bell characters (`\x07`), and backspace (`\x08`) are stripped. This is a **security test** preventing terminal injection attacks. |
| `TestGetSessionForUser` | Creates a session owned by "user-alice". Verifies alice can access it, but "user-bob" gets an error. This is the **IDOR protection test** -- it ensures users can only access their own sessions. |

**Benchmarks:**

| Benchmark Function | What It Measures | Why It Matters |
|-------------------|-----------------|----------------|
| `BenchmarkSessionCreation` | Time to create sessions (includes PTY spawn attempt) | Session creation is the most expensive operation; regressions here affect user experience. |
| `BenchmarkOutputBuffering` | Time to append one output chunk | This runs on every line of Claude CLI output; it must be fast. |
| `BenchmarkGetOutput` | Time to read 100 buffered output chunks | Measures the HTTP response path for the output endpoint. |
| `BenchmarkSessionStatus` | Time to generate a status response | Called frequently by the UI for session health polling. |
| `BenchmarkTimeoutCheck` | Time to check timeouts across 100 sessions | Runs every 60 seconds in production; must handle many sessions efficiently. |

---

## Test File: `internal/server/server_test.go`

**What it validates:** HTTP API endpoints and middleware.

| Test Function | What It Checks |
|--------------|----------------|
| `TestHealthEndpoint` | Sends `GET /health` and verifies a 200 response with `"status": "healthy"`. Also checks for `timestamp`, `active_sessions`, and `memory_alloc_mb` fields. This is what Docker and Kubernetes health checks call. |
| `TestCreateSessionMissingFields` | Sends `POST /api/session/create` with only `{"userId": "test"}` (no credentials). Verifies a 400 Bad Request response. Input validation at the API boundary. |
| `TestCreateSessionValidRequest` | Sends a complete session creation request with userId, credentials, and workspaceType. Accepts either 200 (success) or 500 (Claude CLI not available) -- both are valid outcomes in CI. |
| `TestGetStatusNonExistentSession` | Sends `GET /api/session/non-existent-id/status` with X-User-ID header. Verifies a 404 response for sessions that do not exist. |
| `TestSessionEndpointRequiresUserID` | Sends `GET /api/session/some-id/status` **without** the X-User-ID header. Verifies a 400 response. This tests the IDOR protection middleware -- every session endpoint requires user identification. |
| `TestSendCommandMissingBody` | Sends `POST /api/session/test-id/command` with no request body. Verifies a 400 response. |
| `TestResizeMissingParameters` | Sends a resize request with only `cols` but no `rows`. Verifies a 400 response for incomplete parameters. |
| `TestAuthMiddlewareRejectsNoToken` | Configures the server with `APIAuthToken: "test-secret-token"`, then sends a request **without** an Authorization header. Verifies a 401 Unauthorized response. |
| `TestAuthMiddlewareAcceptsValidToken` | Same server configuration, but sends `Authorization: Bearer test-secret-token`. Verifies the request is not rejected with 401. |

**Benchmarks:**

| Benchmark Function | What It Measures |
|-------------------|-----------------|
| `BenchmarkHealthEndpoint` | Requests/second for the health check endpoint. Should be extremely fast as it is called by load balancers. |
| `BenchmarkCreateSessionRequest` | Request processing time for session creation (including JSON parsing and validation). |
| `BenchmarkGetStatusRequest` | Request processing time for status queries. |

---

## Running Tests in CI

### Basic Test Run

```bash
# Makefile target
make test

# Equivalent to:
go test -v ./...
```

This runs all 23 test functions across the 3 test files. Expected output:

```
=== RUN   TestLoadConfig
--- PASS: TestLoadConfig (0.00s)
=== RUN   TestLoadConfigMissingRequired
--- PASS: TestLoadConfigMissingRequired (0.00s)
...
PASS
ok  github.com/servicenow/claude-terminal-mid-service/internal/config   0.003s
ok  github.com/servicenow/claude-terminal-mid-service/internal/session  0.012s
ok  github.com/servicenow/claude-terminal-mid-service/internal/server   0.008s
```

**Important CI note:** Some tests (like `TestSessionCreation`) gracefully handle the absence of Claude CLI. They log a message and return rather than failing. This means the test suite can run in CI without Claude CLI installed.

### Race Detection Tests

```bash
make test-race

# Equivalent to:
go test -race ./...
```

The race detector is critical for this project because the session manager handles concurrent access from multiple goroutines (HTTP handlers, PTY readers, timeout checker, cleanup routines).

---

## Integration Tests: `scripts/run-tests.sh`

The `scripts/run-tests.sh` script is a comprehensive test runner that goes beyond `go test`. Here is what it does, step by step:

### Phase 1: Environment Setup
```bash
export SERVICENOW_INSTANCE=test.service-now.com
export SERVICENOW_API_USER=test_user
export SERVICENOW_API_PASSWORD=test_password
export NODE_SERVICE_PORT=3000
```
Sets test environment variables so config tests have the required values.

### Phase 2: Unit Tests Per Package
Runs `go test -v` against each package individually:
- `./internal/config`
- `./internal/session`
- `./internal/server`

Tracks pass/fail counts separately.

### Phase 3: Coverage Report
```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out     # Print per-function coverage
go tool cover -html=coverage.out -o coverage.html  # Generate HTML report
```

### Phase 4: Benchmarks
Runs benchmarks per-package and saves results to separate files:
- `benchmark-session.txt`
- `benchmark-server.txt`
- `benchmark-config.txt`

### Phase 5: Race Detection
```bash
go test -race ./...
```

### Phase 6: Static Analysis
```bash
go vet ./...
```

### Phase 7: Format Check
```bash
gofmt -l .
```

### Phase 8: Live Integration Tests
If the HTTP service is running on localhost:3000, it performs live HTTP integration tests:
1. `GET /health` -- verifies the response contains "healthy"
2. `POST /api/session/create` -- attempts to create a session
3. `DELETE /api/session/{id}` -- cleans up the test session

If the service is not running, integration tests are skipped with a warning.

### Phase 9: Summary Report
Prints totals for passed, failed, and coverage percentage. Exits with code 0 if all tests passed, 1 if any failed.

---

## Test Coverage: `make test-coverage`

### How to Generate

```bash
make test-coverage
```

This runs three commands:

```bash
# 1. Run tests and write coverage data to a file
go test -coverprofile=coverage.out ./...

# 2. Print per-function coverage to stdout
go tool cover -func=coverage.out

# 3. Generate an interactive HTML report
go tool cover -html=coverage.out -o coverage.html
```

### How to Interpret Coverage

The `coverage.out` file is machine-readable. The `go tool cover -func` command prints output like:

```
github.com/servicenow/claude-terminal-mid-service/internal/config/config.go:15:   Load          85.7%
github.com/servicenow/claude-terminal-mid-service/internal/session/session.go:42: NewManager    100.0%
github.com/servicenow/claude-terminal-mid-service/internal/session/session.go:78: CreateSession  72.3%
...
total:                                                                                           68.5%
```

Each line shows a function and what percentage of its lines were executed during tests. The `total` line at the bottom is the overall project coverage.

### Coverage in Jenkins

```groovy
stage('Test with Coverage') {
    steps {
        sh 'go test -coverprofile=coverage.out -covermode=atomic ./...'
        sh 'go tool cover -func=coverage.out'
        sh 'go tool cover -html=coverage.out -o coverage.html'

        // Archive the HTML report as a Jenkins artifact
        archiveArtifacts artifacts: 'coverage.html,coverage.out'

        // Optional: fail if coverage drops below a threshold
        sh '''
            COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
            echo "Total coverage: ${COVERAGE}%"
            if [ $(echo "$COVERAGE < 60" | bc) -eq 1 ]; then
                echo "ERROR: Coverage ${COVERAGE}% is below the 60% threshold"
                exit 1
            fi
        '''
    }
}
```

**Note on `-covermode=atomic`**: This flag makes coverage collection safe for concurrent goroutines (which this project heavily uses). Without it, coverage counters could be inaccurate due to race conditions during counting.

---

## Benchmarks: `make bench`

### What They Measure

Benchmarks measure the performance of specific operations in nanoseconds per operation, bytes allocated per operation, and number of allocations per operation.

```bash
# Quick benchmarks
make bench
# Equivalent to: go test -bench=. -benchmem -benchtime=3s ./...

# Full benchmarks with profiling
make bench-full
# Runs: ./scripts/run-benchmarks.sh
```

### Reading Benchmark Output

Example output from `go test -bench=. -benchmem ./internal/session/`:

```
BenchmarkOutputBuffering-8     5000000     312 ns/op     128 B/op     2 allocs/op
BenchmarkGetOutput-8           2000000     856 ns/op     1024 B/op    1 allocs/op
BenchmarkSessionStatus-8       3000000     445 ns/op     512 B/op     5 allocs/op
BenchmarkTimeoutCheck-8          50000   32145 ns/op       0 B/op     0 allocs/op
```

Reading the columns:
- **BenchmarkOutputBuffering-8**: Function name, `-8` means 8 CPU cores
- **5000000**: Number of iterations the benchmark ran
- **312 ns/op**: 312 nanoseconds per operation
- **128 B/op**: 128 bytes allocated per operation
- **2 allocs/op**: 2 heap allocations per operation

### The `run-benchmarks.sh` Script

The `scripts/run-benchmarks.sh` script provides a more thorough benchmark run:

1. **Runs benchmarks per package** with `-benchtime=3s` (3 seconds per benchmark for statistical stability)
2. **Generates CPU profiles** (`-cpuprofile=benchmarks/cpu_*.prof`) showing where CPU time is spent
3. **Generates memory profiles** (`-memprofile=benchmarks/mem_*.prof`) showing where memory is allocated
4. **Compares with baseline** using `benchstat` if a baseline file exists

### Benchmarks in Jenkins

```groovy
stage('Benchmarks') {
    steps {
        sh 'go test -bench=. -benchmem -benchtime=3s -count=5 ./... | tee benchmark-results.txt'
        archiveArtifacts artifacts: 'benchmark-results.txt'
    }
}
```

The `-count=5` flag runs each benchmark 5 times, which is needed by `benchstat` for statistical comparison between builds.

---

## Handling Test Failures in Jenkins

### Exit Codes

Go test commands use standard Unix exit codes:
- **0**: All tests passed
- **1**: One or more tests failed
- **2**: Tests could not be compiled (syntax error, missing dependency)

Jenkins treats any non-zero exit code as a stage failure by default. No special configuration is needed.

### JUnit Reports

Jenkins natively understands JUnit XML format. To generate JUnit-compatible output from Go tests, use the `gotestsum` tool:

```bash
# Install
go install gotest.tools/gotestsum@latest

# Run tests with JUnit output
gotestsum --junitfile test-results.xml -- -v ./...
```

Jenkins pipeline integration:

```groovy
stage('Unit Tests') {
    steps {
        sh 'gotestsum --junitfile test-results.xml -- -v -coverprofile=coverage.out ./...'
    }
    post {
        always {
            junit 'test-results.xml'
        }
    }
}
```

This gives you:
- Test results displayed in the Jenkins UI with pass/fail for each test
- Historical trend graphs showing test pass rates over time
- Click-through to individual test failure logs

### What Happens When a Test Fails

When `TestAuthMiddlewareRejectsNoToken` fails, for example, Jenkins would:
1. Mark the "Unit Tests" stage as FAILED (red)
2. Mark the overall build as FAILED
3. Skip subsequent stages (Docker build, deploy) unless configured otherwise
4. Send a notification (email, Slack) to the developer who pushed the commit
5. Display the test failure in the JUnit report with the error message

---

## Test Parallelism and Ordering

### Package-Level Parallelism

By default, `go test ./...` runs each **package** in parallel (up to `GOMAXPROCS` packages at once), but tests **within** a package run sequentially. This means:

- `internal/config` tests, `internal/session` tests, and `internal/server` tests can run simultaneously
- Within `internal/session`, `TestNewManager` runs before `TestSessionCreation`, etc.

### Implications for CI

1. **No shared state between packages**: Each package gets its own test binary. A variable set in `config_test.go` cannot accidentally affect `session_test.go`. This is safe for parallel execution.

2. **Environment variable isolation**: Tests in this project use `t.Setenv()` which is scoped to the individual test function. Even if two packages run simultaneously and both set `SERVICENOW_INSTANCE`, they do not interfere.

3. **Filesystem isolation**: Tests that create directories (like `TestSessionCreation` creating `/tmp/test-claude-sessions`) could conflict if two packages try to create the same directory. In this project, session tests use unique paths, so this is not an issue.

4. **Port conflicts**: If any tests started HTTP servers on real ports, parallel execution could cause port conflicts. This project uses `httptest.NewRecorder()` (in-memory HTTP testing) rather than real network sockets, avoiding this problem entirely.

### Controlling Parallelism

If tests are flaky due to resource contention (rare for this project), you can limit parallelism:

```bash
# Run packages sequentially (one at a time)
go test -p 1 ./...

# Limit to 2 packages at a time
go test -p 2 ./...
```

In Jenkins, you might limit parallelism if the agent has limited CPU or memory:

```groovy
stage('Unit Tests') {
    steps {
        sh 'go test -v -p 2 ./...'  // Limit to 2 parallel packages
    }
}
```

### Test Ordering Within a Package

Tests within a package run in the order they appear in the source file. However, **you must not rely on this ordering**. Go reserves the right to change test execution order, and the `-shuffle` flag randomizes it:

```bash
# Randomize test order to catch ordering dependencies
go test -shuffle=on ./...
```

Running tests with `-shuffle=on` in CI is a good practice to ensure tests are truly independent. If a test only passes when another test runs before it, `-shuffle` will catch this.
