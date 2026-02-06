# Chapter 11: Your First Jenkinsfile -- Line by Line

This chapter takes the `Jenkinsfile` at the root of the mid-llm-cli project and explains every section in detail. By the end, you will understand what each line does, why it is there, and what happens when it runs.

---

## The Complete Pipeline at a Glance

The Jenkinsfile defines a declarative pipeline with 12 stages:

```
Checkout
   |
Dependencies
   |
Quality Gates (Lint | Format Check | Vet)  <-- parallel
   |
Unit Tests
   |
Race Detection
   |
Coverage
   |
Build Binaries
   |
Docker Build
   |
Integration Tests
   |
Security Scan
   |
Docker Push (main branch only)
   |
Deploy Staging (main branch + parameter)
```

In Jenkins Blue Ocean, this renders as a horizontal flow with three parallel lanes for Quality Gates and sequential stages for everything else. A green circle means the stage passed; red means it failed; grey means it was skipped.

---

## Section 1: `pipeline` and `agent`

```groovy
pipeline {
    agent {
        docker {
            image 'golang:1.24-alpine'
            args '-v go-mod-cache:/go/pkg/mod -v /var/run/docker.sock:/var/run/docker.sock'
        }
    }
```

**`pipeline`**: The top-level block. Everything inside defines a single pipeline. Jenkins parses this as a Groovy DSL.

**`agent { docker { ... } }`**: Tells Jenkins to run every stage inside a Docker container pulled from the `golang:1.24-alpine` image. This means:
- Go 1.24 is pre-installed in the container
- The build environment is identical every time, regardless of what is on the Jenkins host
- No need to install Go on the Jenkins server itself

**`args`**: Passes additional flags to `docker run`:
- `-v go-mod-cache:/go/pkg/mod` -- Creates a named Docker volume for the Go module cache. Without this, every build would re-download all 32+ dependencies from scratch. With it, the `go mod download` step takes seconds instead of minutes.
- `-v /var/run/docker.sock:/var/run/docker.sock` -- Mounts the host's Docker socket into the container. This lets the pipeline run `docker build` and `docker push` commands even though it is itself running inside a container.

---

## Section 2: `environment`

```groovy
    environment {
        GOPATH       = "${WORKSPACE}/.go"
        GOBIN        = "${WORKSPACE}/.go/bin"
        CGO_ENABLED  = '0'
        PATH         = "${GOBIN}:/usr/local/go/bin:${PATH}"

        IMAGE_NAME   = 'claude-terminal-service'
        IMAGE_TAG    = sh(script: 'git rev-parse --short HEAD', returnStdout: true).trim()
        REGISTRY     = credentials('docker-registry-url')

        PROJECT_NAME = 'claude-terminal-mid-service'
    }
```

These environment variables are available to every stage.

| Variable | Value | Purpose |
|----------|-------|---------|
| `GOPATH` | `${WORKSPACE}/.go` | Sets the Go workspace inside the Jenkins workspace. Keeps each build isolated. |
| `GOBIN` | `${WORKSPACE}/.go/bin` | Where `go install` puts compiled tools like `gotestsum` and `golangci-lint`. |
| `CGO_ENABLED` | `0` | Disables CGO for static binaries. Our project has no C dependencies, so this produces smaller, fully static binaries that run on Alpine. |
| `PATH` | `${GOBIN}:...` | Adds `GOBIN` to the path so tools installed with `go install` can be called by name. |
| `IMAGE_NAME` | `claude-terminal-service` | The Docker image name. Used in `docker build` and `docker push` commands. |
| `IMAGE_TAG` | (git short SHA) | The `sh(script: ..., returnStdout: true).trim()` syntax runs a shell command and captures its output. `git rev-parse --short HEAD` gives the 7-character commit hash (e.g., `a1b2c3d`). Every Docker image is tagged with its exact commit. |
| `REGISTRY` | (from credentials) | The `credentials('docker-registry-url')` function reads a Jenkins credential and exposes it as an environment variable. This is a secret text credential containing the Docker registry URL (e.g., `docker.io/yourorg`). |
| `PROJECT_NAME` | `claude-terminal-mid-service` | A human-readable name used in notification messages. |

---

## Section 3: `options`

```groovy
    options {
        timeout(time: 30, unit: 'MINUTES')
        timestamps()
        buildDiscarder(logRotator(numToKeepStr: '20', artifactNumToKeepStr: '5'))
        disableConcurrentBuilds()
        skipDefaultCheckout(true)
    }
```

| Option | What It Does |
|--------|-------------|
| `timeout(30 MINUTES)` | If the entire pipeline takes more than 30 minutes, Jenkins kills it. Prevents stuck builds from blocking the queue forever. |
| `timestamps()` | Adds a timestamp prefix to every line of console output. Example: `[2026-02-06T14:23:01.123Z] Building Claude Terminal Service...` |
| `buildDiscarder(logRotator(...))` | Keeps only the last 20 builds and 5 artifact archives on disk. Without this, Jenkins home grows indefinitely. |
| `disableConcurrentBuilds()` | If a build is running and a new commit is pushed, the new build waits instead of starting in parallel. Prevents resource conflicts. |
| `skipDefaultCheckout(true)` | Disables the automatic `git checkout` that Jenkins does before the first stage. We do it explicitly in the Checkout stage instead, which gives us more control. |

---

## Section 4: `parameters`

```groovy
    parameters {
        choice(
            name: 'DEPLOY_ENV',
            choices: ['none', 'staging', 'production'],
            description: 'Target deployment environment.'
        )
        booleanParam(
            name: 'SKIP_TESTS',
            defaultValue: false,
            description: 'Skip test stages (emergency hotfixes only).'
        )
    }
```

Parameters create form fields on the Jenkins build page. When you click "Build with Parameters" (instead of "Build Now"), you see a dropdown for `DEPLOY_ENV` and a checkbox for `SKIP_TESTS`.

- **DEPLOY_ENV**: Controls whether the pipeline deploys after building. Default is `none` (no deploy). Change to `staging` or `production` to trigger the Deploy stage.
- **SKIP_TESTS**: Emergency escape hatch. When checked, the Quality Gates, Unit Tests, Race Detection, Coverage, and Integration Tests stages are all skipped. Use only for critical hotfixes where you know the change is safe and cannot wait for tests.

Parameters are referenced in stage `when` blocks:

```groovy
when {
    expression { return !params.SKIP_TESTS }
}
```

---

## Section 5: The Stages

### Stage 1: Checkout

```groovy
stage('Checkout') {
    steps {
        checkout scm
        sh 'git log --oneline -5'
    }
}
```

- `checkout scm` clones the repository using the SCM configuration defined in the Jenkins job (which points to the Git repo URL and branch).
- `git log --oneline -5` prints the last 5 commits to the console log. This is purely informational -- it helps you confirm that Jenkins checked out the right branch and commit.

**What success looks like in the console:**
```
Cloning repository https://github.com/yourorg/mid-llm-cli.git
 > git checkout main
a1b2c3d Fix rate limiter cleanup
e4f5g6h Add PostgreSQL session store
...
```

**What failure looks like:** The stage fails if the Git URL is wrong, the branch does not exist, or the Jenkins credential for Git authentication is missing. The error message will say something like `Could not read from remote repository`.

---

### Stage 2: Dependencies

```groovy
stage('Dependencies') {
    steps {
        sh '''
            go mod download
            go mod verify
            go mod tidy
            git diff --exit-code go.mod go.sum || {
                echo "ERROR: go.mod or go.sum changed after tidy."
                exit 1
            }
        '''
    }
}
```

Four commands in sequence:

1. **`go mod download`** -- Downloads all dependencies listed in `go.sum` into the module cache (`/go/pkg/mod`). Because we mounted a Docker volume for this cache, subsequent builds reuse cached modules.

2. **`go mod verify`** -- Checks that the downloaded modules match the expected checksums in `go.sum`. Catches supply chain tampering.

3. **`go mod tidy`** -- Removes unused dependencies and adds missing ones. If a developer added a new import but forgot to run `go mod tidy`, this step catches it.

4. **`git diff --exit-code go.mod go.sum`** -- Verifies that `go mod tidy` did not change anything. If it did, the build fails with a clear error message telling the developer to run `go mod tidy` and commit the changes.

---

### Stage 3: Quality Gates (Parallel)

```groovy
stage('Quality Gates') {
    when {
        expression { return !params.SKIP_TESTS }
    }
    parallel {
        stage('Lint') { ... }
        stage('Format Check') { ... }
        stage('Vet') { ... }
    }
}
```

The `parallel` block runs all three sub-stages simultaneously. In Blue Ocean, this appears as three lanes branching from a single point and converging back. All three must pass for the pipeline to continue.

**Lint** installs `golangci-lint` (a Go meta-linter that runs 50+ checks) and runs it against the entire codebase. It catches issues like unused variables, inefficient code, potential nil pointer dereferences, and style violations.

**Format Check** runs `gofmt -l .` which lists files that are not formatted according to Go's canonical style. If any files are listed, the build fails. This enforces consistent code formatting across the team.

**Vet** runs `go vet ./...` which performs static analysis for common Go mistakes: unreachable code, misused format strings, struct tag errors, and more. This is Go's built-in linter.

---

### Stage 4: Unit Tests

```groovy
stage('Unit Tests') {
    steps {
        sh '''
            go install gotest.tools/gotestsum@latest
            ${GOBIN}/gotestsum \
                --junitfile test-results.xml \
                --format testdox \
                -- -v ./...
        '''
    }
    post {
        always {
            junit testResults: 'test-results.xml', allowEmptyResults: true
        }
    }
}
```

This installs `gotestsum`, a test runner that wraps `go test` and produces JUnit XML output. The `--junitfile test-results.xml` flag generates a file that Jenkins can parse.

The `post { always { junit ... } }` block runs after the stage regardless of pass/fail. Jenkins reads `test-results.xml` and displays test results in the build page with counts of passed/failed/skipped tests.

The tests run against these packages: `./internal/config`, `./internal/session`, `./internal/server` (plus any others that contain `_test.go` files).

---

### Stage 5: Race Detection

```groovy
stage('Race Detection') {
    steps {
        sh 'CGO_ENABLED=1 go test -race ./...'
    }
}
```

Go's race detector instruments the binary to detect concurrent access to shared memory without proper synchronization. This is critical for the mid-llm-cli project because the session manager handles multiple concurrent PTY sessions with goroutines.

Note that `CGO_ENABLED=1` is required because the race detector uses CGO internally. This overrides the `CGO_ENABLED=0` in the environment block for this one command.

---

### Stage 6: Coverage

```groovy
stage('Coverage') {
    steps {
        sh '''
            go test -coverprofile=coverage.out -covermode=atomic ./...
            go tool cover -func=coverage.out | tail -1
            go tool cover -html=coverage.out -o coverage.html
        '''
    }
}
```

- `coverprofile=coverage.out` writes a machine-readable coverage file
- `covermode=atomic` uses atomic counters (safe for concurrent tests)
- `go tool cover -func` prints per-function coverage; `tail -1` shows only the total
- `go tool cover -html` generates an HTML report with green (covered) and red (uncovered) highlighting

The `post` block publishes `coverage.html` as an HTML report linked from the Jenkins build page. You can click "Go Coverage Report" in the build sidebar to view it.

---

### Stage 7: Build Binaries

```groovy
stage('Build Binaries') {
    steps {
        sh '''
            apk add --no-cache make git
            make build
            ls -lh bin/claude-terminal-service bin/ecc-poller
            file bin/claude-terminal-service bin/ecc-poller
        '''
    }
}
```

This runs the same `make build` that developers use locally. The `file` command confirms the binary type (e.g., `ELF 64-bit LSB executable, x86-64, statically linked`). The `post { success { archiveArtifacts ... } }` block saves the binaries as downloadable Jenkins artifacts.

---

### Stage 8: Docker Build

```groovy
stage('Docker Build') {
    steps {
        sh '''
            docker build \
                -t ${IMAGE_NAME}:${IMAGE_TAG} \
                -t ${IMAGE_NAME}:latest \
                --label "git.commit=${IMAGE_TAG}" \
                --label "build.number=${BUILD_NUMBER}" \
                .
        '''
    }
}
```

Builds the multi-stage `Dockerfile` from the project root. The image is tagged with both the git SHA (`a1b2c3d`) and `latest`. Labels embed build metadata into the image for traceability.

---

### Stage 9: Integration Tests

```groovy
stage('Integration Tests') {
    steps {
        sh '''
            docker compose up -d postgres claude-terminal-service
            sleep 15
            # Wait for health check...
            bash scripts/run-tests.sh || true
        '''
    }
    post {
        always {
            sh 'docker compose down --volumes --remove-orphans || true'
        }
    }
}
```

This starts PostgreSQL and the terminal service using the project's `docker-compose.yml`, waits for the health check to pass, and then runs `scripts/run-tests.sh`. The `post { always }` block tears everything down regardless of test results.

The `|| true` after `run-tests.sh` prevents integration test failures from immediately failing the pipeline. The test results are still captured and reported; the stage result reflects the actual test outcome.

---

### Stage 10: Security Scan

```groovy
stage('Security Scan') {
    steps {
        sh '''
            trivy image \
                --severity HIGH,CRITICAL \
                --exit-code 0 \
                --format table \
                ${IMAGE_NAME}:${IMAGE_TAG}
        '''
    }
}
```

Trivy scans the Docker image for known vulnerabilities in OS packages and application dependencies. `--exit-code 0` means the scan reports findings but does not fail the build (informational). Change to `--exit-code 1` if you want HIGH/CRITICAL vulnerabilities to block the build.

---

### Stage 11: Docker Push

```groovy
stage('Docker Push') {
    when {
        branch 'main'
    }
    steps {
        withCredentials([
            usernamePassword(credentialsId: 'docker-registry-creds', ...)
        ]) {
            sh '''
                echo "$DOCKER_PASS" | docker login -u "$DOCKER_USER" --password-stdin ${REGISTRY}
                docker tag ${IMAGE_NAME}:${IMAGE_TAG} ${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}
                docker push ${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}
            '''
        }
    }
}
```

**`when { branch 'main' }`** -- This stage only runs when the pipeline is triggered by a push to the `main` branch. Feature branch builds skip this entirely.

**`withCredentials`** -- Injects the Docker registry username and password from Jenkins credentials as environment variables. The credentials never appear in the console log (Jenkins masks them with `****`).

**`docker login`** -- Uses `--password-stdin` so the password is piped in rather than appearing as a command-line argument (which would be visible in `ps` output).

---

### Stage 12: Deploy Staging

```groovy
stage('Deploy Staging') {
    when {
        allOf {
            branch 'main'
            expression { return params.DEPLOY_ENV == 'staging' || params.DEPLOY_ENV == 'production' }
        }
    }
    steps {
        withCredentials([ ... ]) {
            sh 'echo "Deployment placeholder"'
        }
    }
}
```

This stage requires both conditions: the branch must be `main` AND the `DEPLOY_ENV` parameter must be set to `staging` or `production`. The `withCredentials` block injects ServiceNow API credentials, the encryption key, and the auth token needed by the deployed service.

The deployment commands are placeholder. Replace them with your actual deployment mechanism (kubectl, docker compose pull, ansible, etc.).

---

## Section 6: `post`

```groovy
    post {
        always {
            junit testResults: 'test-results.xml', allowEmptyResults: true
            sh 'docker rmi ${IMAGE_NAME}:${IMAGE_TAG} || true'
            cleanWs()
        }
        success {
            echo "Build SUCCEEDED"
        }
        failure {
            echo "Build FAILED"
        }
    }
```

The `post` block runs after all stages complete:

- **`always`**: Runs no matter what. Publishes test results, cleans up Docker images, and wipes the workspace (`cleanWs()`).
- **`success`**: Runs only if all stages passed. This is where you would enable Slack notifications.
- **`failure`**: Runs only if any stage failed. The commented-out `slackSend` call sends a red alert to your team channel.
- **`unstable`**: Runs if tests had failures but the build itself succeeded.

---

## How to Trigger This Pipeline for the First Time

### Option 1: Manual Trigger

1. In Jenkins, create a new **Pipeline** job
2. Under **Pipeline**, select **Pipeline script from SCM**
3. Set the SCM to **Git** and enter your repository URL
4. Set the branch specifier to `*/main`
5. Set the script path to `Jenkinsfile`
6. Click **Save**
7. Click **Build Now**

### Option 2: Multibranch Pipeline (Recommended)

1. Create a new **Multibranch Pipeline** job
2. Add a **Git** branch source with your repository URL
3. Jenkins automatically discovers branches and creates builds for each branch that contains a `Jenkinsfile`

See [Chapter 13: Webhooks & Triggers](13-webhooks-and-triggers.md) for full details.

---

## How to Read the Console Output

Click on any build number, then click **Console Output** in the left sidebar. You see the raw output of every shell command. Key things to look for:

- **`+ go mod download`** -- Lines prefixed with `+` show the actual commands being executed
- **`--- Running unit tests ---`** -- Echo statements from the pipeline act as section headers
- **`PASS: TestConfigLoading`** -- Individual test results
- **`total: 72.3% of statements`** -- The coverage summary line
- **`ERROR: go.mod changed after tidy`** -- A clear failure message explaining what went wrong

---

## How to Find and Fix a Failing Stage

1. Open the build in Blue Ocean (click the Blue Ocean icon in the sidebar)
2. The failing stage is highlighted in red. Click on it.
3. Read the log output for that stage. The error is usually in the last 10-20 lines.
4. Common failures and fixes:

| Failing Stage | Typical Error | Fix |
|--------------|--------------|-----|
| Dependencies | `go.mod changed after tidy` | Run `go mod tidy` locally, commit the changes |
| Lint | `unused variable 'x'` | Remove the unused variable |
| Format Check | `file.go not formatted` | Run `gofmt -w .` locally, commit |
| Unit Tests | `FAIL: TestSessionCreate` | Fix the failing test or the code it tests |
| Race Detection | `WARNING: DATA RACE` | Add proper mutex locking |
| Build Binaries | `cannot find module` | Check that `go.mod` includes the dependency |
| Docker Build | `COPY failed: file not found` | Ensure the Dockerfile paths match the project layout |
| Security Scan | `CRITICAL: CVE-2024-xxxxx` | Update the affected dependency |

---

Next: [Chapter 12: Jenkins Credentials & Secrets](12-credentials-and-secrets.md)
