# Chapter 4: Code Quality Gates

## What Are Quality Gates in CI/CD?

A quality gate is a checkpoint in your CI/CD pipeline that code must pass before it can proceed to the next stage. Think of it as a series of automated inspections -- if the code fails any inspection, the pipeline stops and reports exactly what went wrong.

Quality gates serve three purposes:

1. **Catch bugs early** -- Finding a race condition in CI is cheaper than finding it in production at 3 AM.
2. **Enforce consistency** -- Every developer on the team follows the same formatting and coding standards, enforced by machines rather than code review opinions.
3. **Prevent regressions** -- Once a codebase passes security scans and linting, quality gates ensure it stays that way.

In Jenkins, each quality gate is typically its own stage. If any stage fails, the pipeline stops, the build is marked as failed, and the developer receives a notification. The stages run in order: formatting first (fastest, cheapest), then linting, then static analysis, then security scanning (slowest, most thorough).

---

## Go Formatting: `gofmt` and `goimports`

### What They Do

Go has an unusual property among programming languages: there is one official formatting style, enforced by a tool that ships with the compiler. There are no debates about tabs vs. spaces or brace placement -- `gofmt` decides, and everyone follows.

**`gofmt`** reformats Go source code to the canonical style. It handles:
- Indentation (tabs, not spaces)
- Brace placement
- Spacing around operators
- Alignment of struct fields

**`goimports`** is a superset of `gofmt` that also manages import statements. It adds missing imports and removes unused ones, and groups them into standard library, external, and internal packages.

### How to Check (Not Fix) in CI

In Jenkins, you do not want to *fix* formatting -- you want to *check* it and fail if it is wrong. This forces developers to format their code before pushing.

```bash
# Check if any files need formatting
# gofmt -l lists files that differ from canonical formatting
# If the output is non-empty, formatting is wrong
UNFORMATTED=$(gofmt -l .)
if [ -n "$UNFORMATTED" ]; then
    echo "The following files are not formatted:"
    echo "$UNFORMATTED"
    exit 1
fi
```

The project's `scripts/run-tests.sh` already includes this check:

```bash
UNFORMATTED=$(gofmt -l . | grep -v vendor || true)
if [ -z "$UNFORMATTED" ]; then
    echo "All files are properly formatted"
else
    echo "The following files need formatting:"
    echo "$UNFORMATTED"
fi
```

The Makefile also provides a `fmt` target that *fixes* formatting (useful for local development, not for CI):

```bash
# Local development: auto-fix formatting
make fmt

# CI: only check, do not modify files
gofmt -l .
```

### Jenkins Stage

```groovy
stage('Format Check') {
    steps {
        sh '''
            UNFORMATTED=$(gofmt -l .)
            if [ -n "$UNFORMATTED" ]; then
                echo "ERROR: The following files are not properly formatted:"
                echo "$UNFORMATTED"
                echo ""
                echo "Run 'gofmt -w .' or 'make fmt' locally and commit the changes."
                exit 1
            fi
            echo "All Go files are properly formatted."
        '''
    }
}
```

---

## Linting: `golangci-lint`

### What It Catches

`golangci-lint` is a meta-linter that runs dozens of individual linters in parallel. It catches issues that the compiler allows but that are almost always bugs or bad practice:

| Category | Examples |
|----------|----------|
| **Unused code** | Unused variables, functions, struct fields, function parameters |
| **Error handling** | Unchecked error returns (the `errcheck` linter) |
| **Style** | Overly complex functions, deeply nested if/else, magic numbers |
| **Performance** | Unnecessary memory allocations, inefficient string concatenation |
| **Bugs** | Copying a mutex, goroutine leaks, incorrect use of `sync` primitives |
| **Deprecated APIs** | Using functions marked as deprecated in their documentation |

### How to Run

The Makefile provides a lint target:

```bash
make lint
```

Which runs:

```bash
golangci-lint run
```

By default, `golangci-lint` looks for a `.golangci.yml` configuration file in the project root. If one does not exist, it runs a sensible set of default linters.

### How to Configure

Create a `.golangci.yml` in the project root to customize which linters run and their settings. A good starting configuration for this project:

```yaml
run:
  timeout: 5m

linters:
  enable:
    - errcheck      # Check for unchecked errors
    - gosimple      # Simplify code
    - govet         # Report suspicious constructs
    - ineffassign   # Detect unused variable assignments
    - staticcheck   # Advanced static analysis
    - unused        # Find unused code
    - gosec         # Security-focused linting
    - bodyclose     # Check HTTP response bodies are closed
    - contextcheck  # Check context.Context usage

linters-settings:
  errcheck:
    check-type-assertions: true
```

### Installation in Jenkins

`golangci-lint` is not part of the Go standard toolchain. The Jenkins agent needs it installed:

```bash
# Install golangci-lint (run once during agent setup or in pipeline)
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.62.0
```

Or in a Docker-based pipeline, use their official image:

```groovy
stage('Lint') {
    agent {
        docker { image 'golangci/golangci-lint:v1.62.0-alpine' }
    }
    steps {
        sh 'golangci-lint run --timeout 5m'
    }
}
```

### Jenkins Stage

```groovy
stage('Lint') {
    steps {
        sh 'golangci-lint run --timeout 5m --out-format checkstyle > lint-report.xml || true'
        recordIssues(tools: [checkStyle(pattern: 'lint-report.xml')])
    }
}
```

The `--out-format checkstyle` flag produces XML output that Jenkins can parse with the Warnings Next Generation plugin, giving you inline annotations on pull requests.

---

## Race Detection: `go test -race`

### What Race Conditions Are

A race condition occurs when two goroutines access the same variable concurrently and at least one of them writes to it. The result is undefined behavior -- the program might work fine most of the time, then crash randomly under load, or silently produce incorrect data.

This project is especially susceptible to race conditions because:
- The session manager (`internal/session/session.go`) is the largest file and manages concurrent PTY sessions
- Multiple goroutines read and write the sessions map
- The output buffer is written by the PTY reader goroutine and read by HTTP handler goroutines
- The ECC poller processes up to 5 items concurrently

### How the Race Detector Works

Go's race detector instruments memory accesses at compile time, adding checks before every read and write. At runtime, it detects when two unsynchronized goroutines access the same memory. When a race is found, it prints a detailed report showing exactly which goroutines are involved and where the conflicting accesses occur.

### How to Run

The Makefile provides a dedicated target:

```bash
make test-race
```

Which runs:

```bash
go test -race ./...
```

The `./...` pattern means "all packages in this module," which includes:
- `./internal/config` -- config_test.go
- `./internal/session` -- session_test.go
- `./internal/server` -- server_test.go
- `./cmd/ecc-poller` -- (no tests currently, but the command would include them if added)

### Why This Matters in CI

Race conditions are non-deterministic. A test might pass 99 times and fail once. The race detector makes these deterministic by detecting the *potential* for a race, even if the race did not actually manifest during that particular test run.

**Important**: The race detector adds significant overhead -- tests run 2-10x slower and use more memory. This is why it runs as a separate stage from the regular test suite, so fast feedback from basic tests is not delayed.

### Jenkins Stage

```groovy
stage('Race Detection') {
    steps {
        sh 'go test -race -timeout 5m ./...'
    }
}
```

The `-timeout 5m` flag prevents the stage from hanging indefinitely if a race causes a deadlock.

---

## Static Analysis: `go vet`

### What It Does

`go vet` is the Go team's official static analysis tool. It examines Go source code and reports constructs that are technically valid Go but are almost certainly mistakes. Unlike a linter, `go vet` focuses exclusively on correctness, not style.

### Common Bugs It Finds

| Bug Type | Example | What `go vet` Reports |
|----------|---------|----------------------|
| **Printf format errors** | `fmt.Printf("%d", "hello")` | Format verb `%d` expects integer, got string |
| **Unreachable code** | Code after a `return` statement | Unreachable code |
| **Copying locks** | `var mu sync.Mutex; mu2 := mu` | Copying a mutex value |
| **Struct tag errors** | `` `json:name` `` (missing quotes) | Struct tag is not in canonical format |
| **Shadowed variables** | Redeclaring `err` in a nested scope | (with shadow analyzer) |
| **Boolean logic** | `if x == true` instead of `if x` | Simplifiable boolean expression |

### How to Run

```bash
go vet ./...
```

The project's `scripts/run-tests.sh` includes this check:

```bash
if go vet ./...; then
    echo "go vet passed"
else
    echo "go vet found issues"
fi
```

### Jenkins Stage

```groovy
stage('Static Analysis') {
    steps {
        sh 'go vet ./...'
    }
}
```

`go vet` is part of the Go standard toolchain, so no extra installation is needed. It runs fast (usually under 10 seconds for this project) and should be one of the first quality gates.

---

## Security Scanning: `gosec` and `govulncheck`

### `gosec` -- Source Code Security Scanner

`gosec` (Go Security Checker) scans Go source code for security vulnerabilities following rules inspired by OWASP and CWE classifications.

**What it catches in this project specifically:**

| Rule | What It Checks | Relevance to This Project |
|------|---------------|--------------------------|
| G101 | Hardcoded credentials | Flags if API tokens or passwords appear in source code |
| G104 | Unhandled errors | Critical for the ServiceNow client -- unhandled HTTP errors could mask failures |
| G110 | Decompression bombs | Relevant if ECC Queue payloads contain compressed data |
| G201 | SQL injection | Relevant for PostgreSQL session store queries |
| G301 | File permissions | Flags overly permissive file creation (workspace directories) |
| G401 | Weak crypto | Would flag if encryption used anything weaker than AES-256 |
| G501 | Insecure TLS | Flags TLS configs that allow old protocol versions |

**How to install and run:**

```bash
# Install
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Run against all packages
gosec ./...

# Output as JSON for Jenkins parsing
gosec -fmt=json -out=gosec-report.json ./...
```

### `govulncheck` -- Known Vulnerability Scanner

`govulncheck` checks your dependencies against the Go vulnerability database. Unlike `gosec` (which scans your source code), `govulncheck` scans your dependency tree for known CVEs.

This is critical for this project because it has dependencies with large attack surfaces:
- `gin` (HTTP framework) -- frequently targeted for request smuggling and injection
- `pgx` (PostgreSQL driver) -- SQL injection vectors
- `crypto` packages -- cryptographic vulnerabilities

**How to install and run:**

```bash
# Install
go install golang.org/x/vuln/cmd/govulncheck@latest

# Run
govulncheck ./...
```

`govulncheck` is smart -- it only reports vulnerabilities in code paths that your project actually calls, not every vulnerability in every dependency. This dramatically reduces false positives.

### Jenkins Stage for Security Scanning

```groovy
stage('Security Scan') {
    steps {
        // Source code security scan
        sh '''
            gosec -fmt=json -out=gosec-report.json -stdout ./... || true
        '''

        // Dependency vulnerability scan
        sh '''
            govulncheck ./... 2>&1 | tee govulncheck-report.txt
        '''

        // Archive reports
        archiveArtifacts artifacts: 'gosec-report.json,govulncheck-report.txt'

        // Fail the build if govulncheck found vulnerabilities
        sh '''
            if govulncheck ./... 2>&1 | grep -q "Vulnerability"; then
                echo "ERROR: Known vulnerabilities found in dependencies"
                exit 1
            fi
        '''
    }
}
```

---

## How Each Quality Gate Maps to Jenkins

Here is the complete picture of how all quality gates fit together in a Jenkins pipeline:

```groovy
pipeline {
    agent {
        docker {
            image 'golang:1.24-alpine'
        }
    }

    stages {
        // Fastest checks first -- fail fast
        stage('Format Check') {
            steps {
                sh '''
                    UNFORMATTED=$(gofmt -l .)
                    if [ -n "$UNFORMATTED" ]; then
                        echo "Unformatted files:"
                        echo "$UNFORMATTED"
                        exit 1
                    fi
                '''
            }
        }

        stage('Vet') {
            steps {
                sh 'go vet ./...'
            }
        }

        stage('Lint') {
            steps {
                sh 'golangci-lint run --timeout 5m'
            }
        }

        stage('Security Scan') {
            steps {
                sh 'gosec ./...'
                sh 'govulncheck ./...'
            }
        }

        // Slowest checks last
        stage('Race Detection') {
            steps {
                sh 'go test -race -timeout 5m ./...'
            }
        }
    }
}
```

**Why this order matters:**

1. **Format Check** (~2 seconds) -- If code is not formatted, everything else is wasted time. Fail immediately.
2. **Vet** (~5 seconds) -- Catches obvious bugs that would make subsequent analysis noisy.
3. **Lint** (~30 seconds) -- More thorough analysis. Depends on code being well-formed (format + vet).
4. **Security Scan** (~45 seconds) -- Dependency scanning requires module download. Source scanning needs compiled code.
5. **Race Detection** (~2-5 minutes) -- Compiles with instrumentation and runs all tests. The slowest gate, so it runs last.

Total quality gate time for this project: approximately 3-7 minutes depending on the Jenkins agent's resources.

---

## Exact Commands Jenkins Would Run

Here is a complete reference of every command, in order, with expected behavior:

```bash
# 1. Format check -- exits non-zero if any file is unformatted
gofmt -l . | (! grep .)

# 2. Static analysis -- exits non-zero if suspicious constructs found
go vet ./...

# 3. Lint -- exits non-zero if linting rules violated
golangci-lint run --timeout 5m

# 4. Source code security scan -- generates report
gosec -fmt=json -out=gosec-report.json ./...

# 5. Dependency vulnerability check -- exits non-zero if vulns found
govulncheck ./...

# 6. Race detection tests -- exits non-zero if races detected
go test -race -timeout 5m ./...
```

Every one of these commands uses exit codes correctly: zero means pass, non-zero means fail. Jenkins interprets non-zero exit codes as stage failures by default, so no special configuration is needed.
