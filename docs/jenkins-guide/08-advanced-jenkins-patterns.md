# Chapter 8: Advanced Jenkins Patterns

## Multibranch Pipeline

A **Multibranch Pipeline** automatically discovers branches, tags, and pull requests in your repository and creates a separate pipeline for each one.

### Why Multibranch?

Without multibranch, you manually create one pipeline per branch. With 10 feature branches, you need 10 pipeline jobs. When a branch is deleted, the job stays around, cluttering Jenkins.

Multibranch solves this:
- **Auto-discovery:** Jenkins scans the repository and creates pipelines for every branch that contains a `Jenkinsfile`
- **Auto-cleanup:** When a branch is deleted from Git, Jenkins removes the corresponding pipeline
- **PR builds:** Pull requests get their own pipelines, with build status posted back to GitHub

### How It Works

```
GitHub Repository                    Jenkins Multibranch Pipeline
+-----------------------+            +---------------------------+
| main (Jenkinsfile)    | ---------> | main       [Build #42]   |
| develop (Jenkinsfile) | ---------> | develop    [Build #18]   |
| feature/websocket     | ---------> | feature/ws [Build #3]    |
| PR #15: fix-session   | ---------> | PR-15      [Build #1]    |
+-----------------------+            +---------------------------+
         |                                       ^
         +-- Branch deleted -------> Job removed |
         +-- New branch created ---> Job created |
```

### Setting Up Multibranch for This Project

**Step 1: Create the Job**

1. Jenkins Dashboard > **New Item**
2. Name: `mid-llm-cli`
3. Select **Multibranch Pipeline**
4. Click OK

**Step 2: Configure Branch Source**

1. Under **Branch Sources**, click **Add source** > **GitHub**
2. Configure:
   - **Credentials:** Select your GitHub credentials (personal access token or GitHub App)
   - **Repository HTTPS URL:** `https://github.com/your-org/mid-llm-cli.git`
   - **Behaviors:**
     - Discover branches: "All branches"
     - Discover pull requests from origin: "Merging the PR with the target branch"
     - Discover pull requests from forks: (disable unless needed)

**Step 3: Configure Build Strategies**

Under **Build Configuration:**
- **Mode:** "by Jenkinsfile"
- **Script Path:** `Jenkinsfile`

Under **Orphaned Item Strategy:**
- **Discard old items:** checked
- **Days to keep:** 7
- **Max items to keep:** 20

This automatically removes pipelines for branches deleted more than 7 days ago.

**Step 4: Configure Scan Triggers**

Under **Scan Multibranch Pipeline Triggers:**
- **Periodically if not otherwise run:** checked
- **Interval:** 1 hour (as a fallback; webhooks handle instant triggers)

### Branch-Specific Behavior in Jenkinsfile

```groovy
pipeline {
    agent { docker { image 'golang:1.24-alpine' } }

    stages {
        // Always run: lint, test, build
        stage('CI') {
            steps {
                sh 'go mod download'
                sh 'golangci-lint run ./...'
                sh 'go test -v ./...'
                sh 'make build'
            }
        }

        // Only on main: build and push Docker image
        stage('Docker') {
            when { branch 'main' }
            steps {
                sh 'docker build -t claude-terminal-service:${GIT_COMMIT} .'
                sh 'docker push registry.example.com/claude-terminal-service:${GIT_COMMIT}'
            }
        }

        // Only on main: deploy to staging
        stage('Deploy Staging') {
            when { branch 'main' }
            steps {
                sh './scripts/deploy.sh staging'
            }
        }

        // Only on PRs: post status and comment
        stage('PR Report') {
            when { changeRequest() }
            steps {
                script {
                    def coverage = sh(script: "go tool cover -func=coverage.out | grep total | awk '{print \$3}'", returnStdout: true).trim()
                    pullRequest.comment("Build passed. Coverage: ${coverage}")
                }
            }
        }
    }
}
```

---

## PR Builds

Pull Request builds are the backbone of code quality enforcement. Every PR triggers a build, and the result is posted back to GitHub as a **status check**.

### The PR Build Flow

```
Developer opens PR
       |
       v
GitHub sends webhook to Jenkins
       |
       v
Jenkins creates pipeline for PR-15
       |
       v
Pipeline runs: lint, test, build
       |
       +--> PASS: Green check on GitHub PR
       +--> FAIL: Red X on GitHub PR, blocks merge
```

### GitHub Commit Status

Jenkins can post build status directly to GitHub, showing as checks on the PR page:

```groovy
pipeline {
    stages {
        stage('Lint') {
            steps {
                // Set status to "pending" at stage start
                githubNotify(
                    status: 'PENDING',
                    context: 'ci/lint',
                    description: 'Linting in progress...'
                )
                sh 'golangci-lint run ./...'
            }
            post {
                success {
                    githubNotify(status: 'SUCCESS', context: 'ci/lint', description: 'Lint passed')
                }
                failure {
                    githubNotify(status: 'FAILURE', context: 'ci/lint', description: 'Lint failed')
                }
            }
        }

        stage('Tests') {
            steps {
                githubNotify(status: 'PENDING', context: 'ci/tests', description: 'Tests running...')
                sh 'go test -v ./...'
            }
            post {
                success {
                    githubNotify(status: 'SUCCESS', context: 'ci/tests', description: 'All tests passed')
                }
                failure {
                    githubNotify(status: 'FAILURE', context: 'ci/tests', description: 'Tests failed')
                }
            }
        }
    }
}
```

On the GitHub PR page, this appears as:

```
Checks:
  ci/lint   -- All checks have passed       [green checkmark]
  ci/tests  -- All tests passed (47/47)     [green checkmark]
```

---

## Branch Protection

Branch protection rules enforce that Jenkins builds must pass before code can be merged.

### Setting Up Branch Protection on GitHub

1. Go to your repository > **Settings** > **Branches**
2. Under **Branch protection rules**, click **Add rule**
3. Configure:
   - **Branch name pattern:** `main`
   - **Require status checks to pass before merging:** checked
   - **Status checks that are required:**
     - `ci/lint`
     - `ci/tests`
     - `ci/build`
   - **Require branches to be up to date before merging:** checked (ensures the PR is rebased)
   - **Require pull request reviews before merging:** checked (optional but recommended)

### How It Looks in Practice

```
PR #15: Fix session timeout handling

Changes: internal/session/session.go (+12, -3)

Status Checks:
  [PASS] ci/lint       -- Lint passed
  [PASS] ci/tests      -- All tests passed (47/47)
  [PASS] ci/build      -- Build succeeded
  [PASS] ci/coverage   -- Coverage: 74.2% (above 70% threshold)

Reviews:
  [APPROVED] @reviewer1

Merge button: [ENABLED - Squash and merge]
```

If any check fails:

```
Status Checks:
  [PASS] ci/lint       -- Lint passed
  [FAIL] ci/tests      -- 2 tests failed
  [PASS] ci/build      -- Build succeeded

Merge button: [DISABLED - Required status check "ci/tests" is failing]
```

---

## Quality Gates

Quality gates are automated checks that block the pipeline when quality criteria are not met.

### Coverage Threshold Gate

```groovy
stage('Coverage Gate') {
    steps {
        sh 'go test -coverprofile=coverage.out ./...'
        script {
            def coverageOutput = sh(
                script: "go tool cover -func=coverage.out | grep total | awk '{print \$3}' | sed 's/%//'",
                returnStdout: true
            ).trim()
            def coverage = coverageOutput.toFloat()

            echo "Total test coverage: ${coverage}%"

            // Store for display
            currentBuild.description = "Coverage: ${coverage}%"

            // Fail if below threshold
            if (coverage < 70.0) {
                unstable("Coverage ${coverage}% is below the 70% threshold")
            }
        }
    }
}
```

### Binary Size Gate

```groovy
stage('Binary Size Check') {
    steps {
        sh 'make build'
        script {
            def serverSize = sh(script: "stat -f%z bin/claude-terminal-service || stat -c%s bin/claude-terminal-service", returnStdout: true).trim().toLong()
            def sizeMB = serverSize / (1024 * 1024)
            echo "Server binary: ${sizeMB} MB"

            if (sizeMB > 50) {
                error("Binary size ${sizeMB}MB exceeds 50MB limit. Check for embedded assets or debug symbols.")
            }
        }
    }
}
```

### Dependency Vulnerability Gate

```groovy
stage('Security Gate') {
    steps {
        sh 'govulncheck ./... > govulncheck-report.txt 2>&1 || true'
        script {
            def report = readFile('govulncheck-report.txt')
            if (report.contains('Vulnerability #')) {
                def count = (report =~ /Vulnerability #/).count
                unstable("Found ${count} known vulnerabilities. Review govulncheck-report.txt")
            } else {
                echo "No known vulnerabilities found"
            }
        }
    }
}
```

---

## Notifications

### Slack Notifications

**Plugin required:** Slack Notification Plugin

**Setup:**
1. In Slack, create an Incoming Webhook for your channel
2. In Jenkins: **Manage Jenkins** > **Configure System** > **Slack**
3. Add the webhook URL and default channel

**Usage:**
```groovy
post {
    success {
        slackSend(
            channel: '#mid-llm-cli-ci',
            color: 'good',
            message: """
                :white_check_mark: *Build Succeeded*
                Job: ${env.JOB_NAME} #${env.BUILD_NUMBER}
                Branch: ${env.BRANCH_NAME}
                Commit: ${env.GIT_COMMIT?.take(7)}
                Duration: ${currentBuild.durationString}
                <${env.BUILD_URL}|View Build>
            """.stripIndent().trim()
        )
    }
    failure {
        slackSend(
            channel: '#mid-llm-cli-ci',
            color: 'danger',
            message: """
                :x: *Build Failed*
                Job: ${env.JOB_NAME} #${env.BUILD_NUMBER}
                Branch: ${env.BRANCH_NAME}
                Commit: ${env.GIT_COMMIT?.take(7)}
                Stage: ${env.STAGE_NAME}
                <${env.BUILD_URL}console|View Logs>
            """.stripIndent().trim()
        )
    }
}
```

### Email Notifications

```groovy
post {
    failure {
        emailext(
            subject: "FAILED: ${env.JOB_NAME} #${env.BUILD_NUMBER}",
            body: """
                <h2>Build Failed</h2>
                <p><strong>Job:</strong> ${env.JOB_NAME}</p>
                <p><strong>Build:</strong> #${env.BUILD_NUMBER}</p>
                <p><strong>Branch:</strong> ${env.BRANCH_NAME}</p>
                <p><strong>Console:</strong> <a href="${env.BUILD_URL}console">View Logs</a></p>
            """,
            mimeType: 'text/html',
            recipientProviders: [
                culprits(),        // People who committed since last successful build
                requestor()        // Person who triggered the build
            ]
        )
    }
}
```

### GitHub Commit Status

```groovy
post {
    success {
        githubNotify(
            status: 'SUCCESS',
            context: 'ci/jenkins',
            description: "Build #${BUILD_NUMBER} passed",
            targetUrl: "${BUILD_URL}"
        )
    }
    failure {
        githubNotify(
            status: 'FAILURE',
            context: 'ci/jenkins',
            description: "Build #${BUILD_NUMBER} failed",
            targetUrl: "${BUILD_URL}"
        )
    }
}
```

---

## Build Artifacts

### Archiving Binaries and Reports

```groovy
post {
    always {
        // Archive Go binaries
        archiveArtifacts(
            artifacts: 'bin/claude-terminal-service, bin/ecc-poller',
            fingerprint: true,           // Track where this artifact is used
            allowEmptyArchive: false      // Fail if binaries are missing
        )

        // Archive coverage report
        archiveArtifacts(
            artifacts: 'coverage.html, coverage.out',
            allowEmptyArchive: true       // OK if tests were skipped
        )

        // Archive security scan
        archiveArtifacts(
            artifacts: 'trivy-report.json, govulncheck-report.txt',
            allowEmptyArchive: true
        )

        // Publish JUnit test results
        junit(
            testResults: 'test-results/*.xml',
            allowEmptyResults: true
        )
    }
}
```

### Docker Image Digest

```groovy
stage('Docker Push') {
    steps {
        script {
            def digest = sh(
                script: "docker push ${REGISTRY}/${APP_NAME}:${IMAGE_TAG} 2>&1 | grep digest | awk '{print \$3}'",
                returnStdout: true
            ).trim()

            echo "Pushed image with digest: ${digest}"

            // Write digest to a file for archival
            writeFile file: 'image-digest.txt', text: "${REGISTRY}/${APP_NAME}@${digest}"
            archiveArtifacts artifacts: 'image-digest.txt'

            // Set as build description
            currentBuild.description = "Image: ${IMAGE_TAG} (${digest.take(12)})"
        }
    }
}
```

---

## Caching Strategies

Caching is critical for build speed. Without caches, every build downloads dependencies, tools, and base images from scratch.

### Go Module Cache

Go modules are downloaded to `$GOPATH/pkg/mod/`. Persisting this directory between builds avoids re-downloading hundreds of megabytes of dependencies.

**Method 1: Docker Volume Mount**

```groovy
pipeline {
    agent {
        docker {
            image 'golang:1.24-alpine'
            args '-v go-mod-cache:/go/pkg/mod -v go-build-cache:/root/.cache/go-build'
        }
    }
}
```

The named volumes `go-mod-cache` and `go-build-cache` persist on the Docker host between builds.

**Method 2: Agent Directory**

If not using Docker agents:

```groovy
environment {
    GOPATH = "${WORKSPACE}/.go"
    GOMODCACHE = "${HOME}/.cache/go-mod"
}

stage('Dependencies') {
    steps {
        sh 'go mod download'
    }
}
```

The `${HOME}/.cache/go-mod` directory persists on the agent between builds.

**Impact:**
```
Cold build (no cache):  go mod download ... 45 seconds
Warm build (cached):    go mod download ... 2 seconds
```

### Docker Layer Cache

Docker images are built layer by layer. If a layer has not changed, Docker reuses the cached version.

**Method 1: BuildKit Cache Mount**

```groovy
stage('Docker Build') {
    steps {
        sh '''
            DOCKER_BUILDKIT=1 docker build \
                --cache-from ${REGISTRY}/${APP_NAME}:latest \
                --build-arg BUILDKIT_INLINE_CACHE=1 \
                -t ${REGISTRY}/${APP_NAME}:${IMAGE_TAG} \
                .
        '''
    }
}
```

`--cache-from` tells Docker to pull the previous image's layers and reuse them. The `BUILDKIT_INLINE_CACHE=1` arg embeds cache metadata in the image.

**Method 2: Multi-stage Cache**

```groovy
stage('Docker Build') {
    steps {
        sh '''
            # Pull previous image for layer cache
            docker pull ${REGISTRY}/${APP_NAME}:latest || true

            # Build with cache
            docker build \
                --cache-from ${REGISTRY}/${APP_NAME}:latest \
                -t ${REGISTRY}/${APP_NAME}:${IMAGE_TAG} \
                -t ${REGISTRY}/${APP_NAME}:latest \
                .
        '''
    }
}
```

**Impact:**
```
Cold build (no cache):  docker build ... 4 minutes 30 seconds
  - Download golang:1.24-alpine ... 45s
  - go mod download ... 40s
  - make build ... 30s
  - npm install claude-code ... 60s

Warm build (cached):    docker build ... 45 seconds
  - All layers cached except COPY and build steps
```

### Tool Cache

Tools like `golangci-lint` should not be downloaded on every build.

```groovy
stage('Install Tools') {
    steps {
        sh '''
            # Only download if not already cached
            if ! command -v golangci-lint &> /dev/null; then
                wget -qO- https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
                    | sh -s -- -b /usr/local/bin v1.61.0
            fi
            golangci-lint --version
        '''
    }
}
```

With Docker agents, pre-bake tools into a custom CI image:

```dockerfile
# Dockerfile.ci
FROM golang:1.24-alpine

RUN apk add --no-cache git make curl docker-cli
RUN wget -qO- https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
    | sh -s -- -b /usr/local/bin v1.61.0
RUN go install golang.org/x/vuln/cmd/govulncheck@latest
```

```groovy
pipeline {
    agent {
        docker {
            image 'your-registry/golang-ci:1.24'   // Custom image with tools pre-installed
        }
    }
}
```

---

## Parallel Test Execution

For large test suites, split tests across multiple agents to reduce total execution time.

### Split by Package

```groovy
stage('Tests') {
    parallel {
        stage('Session Tests') {
            agent { docker { image 'golang:1.24-alpine' } }
            steps {
                sh 'go test -v ./internal/session/...'
            }
        }
        stage('Server Tests') {
            agent { docker { image 'golang:1.24-alpine' } }
            steps {
                sh 'go test -v ./internal/server/...'
            }
        }
        stage('Crypto Tests') {
            agent { docker { image 'golang:1.24-alpine' } }
            steps {
                sh 'go test -v ./internal/crypto/... ./internal/middleware/...'
            }
        }
        stage('Integration Tests') {
            agent { label 'docker-host' }
            steps {
                sh 'docker compose -f docker-compose.test.yml up -d postgres'
                sh 'go test -v -tags=integration ./...'
                sh 'docker compose -f docker-compose.test.yml down -v'
            }
        }
    }
}
```

### Dynamic Test Splitting

For projects with hundreds of test packages, dynamically split them:

```groovy
stage('Tests') {
    steps {
        script {
            // Get all test packages
            def packages = sh(
                script: "go list ./... | grep -v /vendor/",
                returnStdout: true
            ).trim().split('\n')

            // Split into N chunks
            def chunks = packages.collate(Math.ceil(packages.size() / 3) as int)
            def parallelStages = [:]

            chunks.eachWithIndex { chunk, i ->
                parallelStages["Tests Part ${i+1}"] = {
                    node('golang') {
                        checkout scm
                        sh "go test -v ${chunk.join(' ')}"
                    }
                }
            }

            parallel parallelStages
        }
    }
}
```

---

## Pipeline Visualization: Blue Ocean

**Blue Ocean** is a Jenkins plugin that provides a modern, visual interface for pipelines.

### What Blue Ocean Shows

```
+----------+     +------+     +--------------------+     +-------+     +-------+
| Checkout |---->| Deps |---->| Quality Checks     |---->| Tests |---->| Build |
| (5s)     |     | (15s)|     | +------+ +---+     |     | (90s) |     | (30s) |
|  [PASS]  |     |[PASS]|     | | Lint | |Vet|     |     | [PASS]|     | [PASS]|
+----------+     +------+     | |[PASS]| |[P]|     |     +-------+     +-------+
                               | +------+ +---+     |
                               | +--------+         |
                               | |Format  |         |
                               | | [PASS] |         |
                               +--------------------+
```

Each stage is a column. Parallel stages are shown as rows within a column. Colors indicate status:
- **Blue:** In progress
- **Green:** Passed
- **Red:** Failed
- **Gray:** Skipped (via `when` condition)

### Installing Blue Ocean

1. **Manage Jenkins** > **Manage Plugins** > **Available**
2. Search for "Blue Ocean"
3. Install "Blue Ocean" (the aggregator plugin that installs all sub-plugins)
4. Restart Jenkins
5. Access Blue Ocean at `http://jenkins:8080/blue`

### Blue Ocean Features

| Feature | Description |
|---------|-------------|
| **Pipeline Editor** | Visual drag-and-drop pipeline creation (generates Jenkinsfile) |
| **Branch View** | Shows all branches and their build status in one view |
| **PR View** | Shows all pull requests and their test results |
| **Log Viewer** | Step-by-step log output with expandable sections |
| **Favorites** | Pin frequently accessed pipelines to your dashboard |

---

## Putting It All Together

Here is how these advanced patterns combine for the Claude Terminal MID Service:

```
Multibranch Pipeline: mid-llm-cli
  |
  +-- main
  |     |-- Build #42 [SUCCESS]
  |     |     Stage: Checkout -> Deps -> [Lint|Format|Vet] -> Tests -> Build -> Docker -> Push -> Deploy Staging
  |     |     Artifacts: binaries, coverage.html, trivy-report.json
  |     |     Notifications: Slack (green), GitHub status (success)
  |     |
  |     +-- Build #43 [WAITING: Deploy Production]
  |           Input: "Deploy abc1234 to production?" [Approve] [Abort]
  |
  +-- develop
  |     +-- Build #18 [SUCCESS]
  |           Stage: Checkout -> Deps -> [Lint|Format|Vet] -> Tests -> Build -> Docker
  |           (No deployment -- develop branch)
  |
  +-- feature/websocket
  |     +-- Build #3 [FAILURE]
  |           Stage: Checkout -> Deps -> [Lint: FAIL]
  |           Error: "unreachable code after return statement"
  |           Notification: Slack (red), email to committer
  |
  +-- PR-15: fix-session-timeout
        +-- Build #1 [SUCCESS]
              Stage: Checkout -> Deps -> [Lint|Format|Vet] -> Tests -> Build
              GitHub: Check "ci/jenkins" = SUCCESS
              Coverage: 74.2% (above 70% threshold)
```

---

## Summary

Advanced Jenkins patterns transform a basic pipeline into a production-grade CI/CD system:

- **Multibranch Pipelines** auto-discover branches and PRs, creating and cleaning up pipelines automatically
- **PR Builds** with GitHub status checks enforce code quality before merge
- **Branch protection** prevents merging code that fails CI
- **Quality gates** block deployment when coverage drops or vulnerabilities are found
- **Notifications** keep the team informed via Slack, email, and GitHub
- **Caching** (Go modules, Docker layers, tools) cuts build times from minutes to seconds
- **Parallel execution** splits work across agents for faster feedback
- **Blue Ocean** provides visual pipeline monitoring

---

**Previous:** [Chapter 7: Pipeline Syntax Deep Dive](07-pipeline-syntax-deep-dive.md)
**Next:** [Chapter 9: Best Practices and Pitfalls](09-best-practices-and-pitfalls.md)
