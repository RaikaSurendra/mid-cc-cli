# Chapter 7: Jenkins Pipeline Syntax Deep Dive

## Two Pipeline Syntaxes

Jenkins supports two ways to write pipelines: **Declarative** and **Scripted**. Both produce the same result — a series of stages that build, test, and deploy your code. The difference is in how you write them.

### Quick Comparison

| Aspect | Declarative | Scripted |
|--------|-------------|----------|
| **Syntax** | Structured, opinionated | Freeform Groovy |
| **Learning curve** | Easier — looks like configuration | Harder — requires Groovy knowledge |
| **Flexibility** | Covers 90% of use cases | Covers 100% of use cases |
| **Error messages** | Clear (syntax validation) | Cryptic (Groovy stack traces) |
| **Best for** | Standard CI/CD pipelines | Complex, dynamic logic |
| **Recommendation** | Start here | Escape hatch for edge cases |

### Declarative Example (This Project)

```groovy
pipeline {
    agent {
        docker { image 'golang:1.24-alpine' }
    }
    stages {
        stage('Build') {
            steps {
                sh 'make build'
            }
        }
    }
}
```

### Scripted Example (Same Build)

```groovy
node {
    docker.image('golang:1.24-alpine').inside {
        stage('Build') {
            sh 'make build'
        }
    }
}
```

**Rule of thumb:** Use Declarative unless you need something it cannot express (dynamic stage generation, complex conditionals, try/catch blocks). Even then, you can embed Scripted blocks inside Declarative with the `script { }` step.

---

## Declarative Pipeline Anatomy

Every Declarative pipeline follows this structure:

```groovy
pipeline {
    agent { ... }          // WHERE to run
    environment { ... }    // Variables available to all stages
    options { ... }        // Build behavior settings
    parameters { ... }     // User inputs (shown before build starts)
    triggers { ... }       // Automatic build triggers

    stages {               // THE WORK
        stage('Name') {
            steps { ... }
        }
    }

    post { ... }           // Cleanup and notifications (runs after stages)
}
```

Let's explore each directive using the Claude Terminal MID Service as our example.

---

## `agent` -- Where to Run

The `agent` directive tells Jenkins where the pipeline (or a specific stage) should execute.

### Agent Types

#### `any` -- Run on Any Available Agent

```groovy
pipeline {
    agent any    // Jenkins picks the first available agent
    stages { ... }
}
```

Use this for simple pipelines where the agent does not matter.

#### `none` -- No Default Agent (Set Per Stage)

```groovy
pipeline {
    agent none   // Each stage must declare its own agent
    stages {
        stage('Build') {
            agent { docker { image 'golang:1.24-alpine' } }
            steps { sh 'make build' }
        }
        stage('Docker Build') {
            agent { label 'docker-host' }
            steps { sh 'docker build -t my-image .' }
        }
    }
}
```

Use this when different stages need different environments.

#### `docker` -- Run Inside a Docker Container

```groovy
pipeline {
    agent {
        docker {
            image 'golang:1.24-alpine'
            args '-v /go/pkg/mod:/go/pkg/mod'   // Mount Go module cache
        }
    }
    stages { ... }
}
```

This is the recommended approach for this project. Every build runs in a clean `golang:1.24-alpine` container, ensuring consistent Go versions and dependencies.

**How it works internally:**
1. Jenkins pulls the `golang:1.24-alpine` image
2. Starts a container with the workspace mounted at `/workspace`
3. Runs all `sh` commands inside this container
4. Destroys the container when the pipeline finishes

#### `label` -- Run on Agent with Matching Label

```groovy
pipeline {
    agent { label 'linux && docker' }
    stages { ... }
}
```

Labels are tags assigned to agents. `label 'linux && docker'` means "only run on agents that have both the `linux` and `docker` labels."

#### `dockerfile` -- Build Agent from a Dockerfile

```groovy
pipeline {
    agent {
        dockerfile {
            filename 'Dockerfile.ci'       // Custom CI Dockerfile
            dir 'build'                    // Directory containing the Dockerfile
            additionalBuildArgs '--build-arg GO_VERSION=1.24'
        }
    }
    stages { ... }
}
```

This builds a Docker image from a Dockerfile in the repo, then runs the pipeline inside it. Useful when the CI environment needs custom tools.

### Per-Stage Agent Override

```groovy
pipeline {
    agent { docker { image 'golang:1.24-alpine' } }   // Default for most stages
    stages {
        stage('Lint') {
            steps { sh 'golangci-lint run ./...' }     // Uses default agent
        }
        stage('Docker Build') {
            agent { label 'docker-host' }              // Override: needs Docker daemon
            steps { sh 'docker build -t claude-terminal-service .' }
        }
    }
}
```

---

## `environment` -- Variables

The `environment` directive defines environment variables available to all stages.

### Pipeline-Level Variables

```groovy
pipeline {
    agent { docker { image 'golang:1.24-alpine' } }

    environment {
        // Go build settings
        CGO_ENABLED   = '0'
        GOOS          = 'linux'
        GOARCH        = 'amd64'
        GOPATH        = "${WORKSPACE}/.go"

        // Project settings
        APP_NAME      = 'claude-terminal-service'
        BINARY_DIR    = 'bin'

        // Docker settings
        IMAGE_NAME    = 'claude-terminal-service'
        IMAGE_TAG     = "${env.GIT_COMMIT?.take(7) ?: 'latest'}"
        REGISTRY      = 'registry.example.com/mid-llm-cli'

        // Credentials (pulled from Jenkins credential store -- never hardcoded)
        DOCKER_CREDS  = credentials('docker-registry-credentials')
        SN_API_TOKEN  = credentials('servicenow-api-token')
    }

    stages { ... }
}
```

### Stage-Level Variables

```groovy
stage('Integration Tests') {
    environment {
        DB_HOST     = 'localhost'
        DB_PORT     = '5432'
        DB_USER     = 'test_user'
        DB_PASSWORD = 'test_password'
        DB_NAME     = 'test_claude_terminal'
    }
    steps {
        sh 'go test -v -tags=integration ./...'
    }
}
```

Stage-level variables override pipeline-level variables and are only available within that stage.

### The `credentials()` Helper

Jenkins manages secrets (passwords, API keys, SSH keys) in its credential store. The `credentials()` helper injects them as environment variables:

```groovy
environment {
    // For "Username with password" credentials:
    DOCKER_CREDS = credentials('docker-registry-credentials')
    // This creates THREE variables:
    //   DOCKER_CREDS       = 'username:password'
    //   DOCKER_CREDS_USR   = 'username'
    //   DOCKER_CREDS_PSW   = 'password'

    // For "Secret text" credentials:
    API_TOKEN = credentials('servicenow-api-token')
    // This creates ONE variable:
    //   API_TOKEN = 'the-secret-value'
}
```

Jenkins automatically masks these values in build logs — if the password appears in console output, it shows `****` instead.

---

## `options` -- Build Behavior

```groovy
pipeline {
    options {
        // Abort the build if it runs longer than 30 minutes
        timeout(time: 30, unit: 'MINUTES')

        // Keep only the last 20 builds (saves disk space)
        buildDiscarder(logRotator(numToKeepStr: '20'))

        // Prepend timestamps to every console output line
        timestamps()

        // Skip checking out code in every stage (we do it once explicitly)
        skipDefaultCheckout()

        // Do not allow concurrent builds of the same pipeline
        disableConcurrentBuilds()

        // Retry the entire pipeline up to 2 times on failure
        retry(2)
    }
    stages { ... }
}
```

### Why These Matter for This Project

| Option | Reason |
|--------|--------|
| `timeout(30)` | Docker builds can hang if the npm registry is unreachable. A 30-minute timeout prevents stuck builds from blocking the queue. |
| `buildDiscarder(20)` | Each build archives binaries and Docker images. Without cleanup, the Jenkins disk fills up within weeks. |
| `timestamps()` | When debugging a slow integration test, timestamps show exactly which step took 90 seconds. |
| `disableConcurrentBuilds()` | Two simultaneous Docker pushes with the `latest` tag would race. One at a time. |

---

## `parameters` -- User Inputs

Parameters create a form that appears before the build starts (or are passed via the API).

```groovy
pipeline {
    parameters {
        // Drop-down: which environment to deploy to
        choice(
            name: 'DEPLOY_ENV',
            choices: ['staging', 'production'],
            description: 'Target deployment environment'
        )

        // Checkbox: skip tests for emergency hotfix
        booleanParam(
            name: 'SKIP_TESTS',
            defaultValue: false,
            description: 'Skip tests (emergency hotfix only)'
        )

        // Text field: specific Git tag to build
        string(
            name: 'GIT_TAG',
            defaultValue: '',
            description: 'Specific Git tag to build (leave empty for HEAD)'
        )

        // Password field: one-time token
        password(
            name: 'DEPLOY_TOKEN',
            description: 'One-time deployment authorization token'
        )
    }

    stages {
        stage('Test') {
            when {
                expression { return !params.SKIP_TESTS }
            }
            steps {
                sh 'go test -v ./...'
            }
        }

        stage('Deploy') {
            steps {
                echo "Deploying to ${params.DEPLOY_ENV}..."
            }
        }
    }
}
```

**Important:** Parameters are available as `params.PARAM_NAME` in the pipeline. They persist between builds — if you ran the last build with `DEPLOY_ENV=production`, the next build's form pre-fills with that value.

---

## `triggers` -- Automatic Build Triggers

```groovy
pipeline {
    triggers {
        // Poll SCM every 5 minutes for changes
        pollSCM('H/5 * * * *')

        // Cron schedule: nightly build at 2 AM
        cron('H 2 * * *')

        // Trigger when another job completes
        upstream(upstreamProjects: 'shared-library-build', threshold: hudson.model.Result.SUCCESS)
    }
    stages { ... }
}
```

**Note:** Webhook triggers (GitHub, GitLab) are configured in the job settings, not in the Jenkinsfile. Webhooks are preferred over polling because they are instant and do not waste resources. See Chapter 13 for webhook setup.

**The `H` in cron expressions:** Jenkins replaces `H` with a hash of the job name. `H/5 * * * *` means "every 5 minutes, but the exact minute offset depends on the job name." This spreads load across the cluster — 50 jobs all polling at `*/5` would create a thundering herd at :00, :05, :10, etc.

---

## `stages` and `steps` -- The Work

### Basic Stage

```groovy
stages {
    stage('Build Server') {
        steps {
            sh 'CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/claude-terminal-service ./cmd/server'
        }
    }
    stage('Build Poller') {
        steps {
            sh 'CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/ecc-poller ./cmd/ecc-poller'
        }
    }
}
```

### Common Step Types

```groovy
steps {
    // Run a shell command
    sh 'make build'

    // Run a shell command and capture output
    script {
        def version = sh(script: 'go version', returnStdout: true).trim()
        echo "Building with: ${version}"
    }

    // Change directory for this step
    dir('internal/session') {
        sh 'go test -v .'
    }

    // Retry a flaky step
    retry(3) {
        sh 'curl -f http://staging.example.com/health'
    }

    // Wait between retries
    retry(3) {
        sleep(time: 10, unit: 'SECONDS')
        sh 'curl -f http://staging.example.com/health'
    }

    // Write a file
    writeFile file: 'test-config.json', text: '{"db_host": "localhost"}'

    // Archive artifacts
    archiveArtifacts artifacts: 'bin/*', fingerprint: true

    // Publish test results (JUnit format)
    junit 'test-results/*.xml'

    // Copy artifacts from another build
    copyArtifacts projectName: 'mid-llm-cli-nightly', filter: 'bin/*'
}
```

### Parallel Stages

```groovy
stage('Quality Checks') {
    parallel {
        stage('Lint') {
            steps {
                sh 'golangci-lint run ./...'
            }
        }
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
    }
}
```

**Important:** Parallel stages run simultaneously on the same agent (using separate threads). If you need them on different agents, use `agent` within each parallel stage.

### The `script` Block -- Escape Hatch to Scripted Pipeline

When Declarative syntax is not enough, embed arbitrary Groovy inside `script { }`:

```groovy
stage('Coverage Gate') {
    steps {
        sh 'go test -coverprofile=coverage.out ./...'
        script {
            def output = sh(
                script: "go tool cover -func=coverage.out | grep total | awk '{print \$3}' | sed 's/%//'",
                returnStdout: true
            ).trim()
            def coverage = output.toFloat()

            echo "Total coverage: ${coverage}%"

            if (coverage < 70.0) {
                error("Coverage ${coverage}% is below the 70% threshold")
            }

            // Set a build description for the Jenkins UI
            currentBuild.description = "Coverage: ${coverage}%"
        }
    }
}
```

### Input -- Human Approval Gate

```groovy
stage('Deploy Production') {
    input {
        message "Deploy version ${IMAGE_TAG} to production?"
        ok "Yes, deploy to production"
        submitter "admin,deploy-team"
        parameters {
            string(name: 'CONFIRM', defaultValue: '', description: 'Type "deploy" to confirm')
        }
    }
    steps {
        script {
            if (params.CONFIRM != 'deploy') {
                error('Deployment confirmation text did not match')
            }
        }
        sh './scripts/deploy.sh production'
    }
}
```

The `input` directive pauses the pipeline and shows a form in the Jenkins UI. The build sits idle (not consuming an executor) until someone approves or the timeout expires.

---

## `when` -- Conditional Execution

The `when` directive controls whether a stage runs based on conditions.

### Branch Conditions

```groovy
stage('Deploy Staging') {
    when {
        branch 'main'    // Only run when building the 'main' branch
    }
    steps {
        sh './scripts/deploy.sh staging'
    }
}

stage('PR Validation') {
    when {
        changeRequest()   // Only run on pull requests
    }
    steps {
        sh 'go test -v ./...'
        // Post PR comment with test results
    }
}
```

### Expression Conditions

```groovy
stage('Nightly Security Scan') {
    when {
        expression { return env.BUILD_CAUSE == 'TIMERTRIGGER' }
    }
    steps {
        sh 'trivy image claude-terminal-service:latest'
    }
}
```

### Combined Conditions

```groovy
stage('Deploy Production') {
    when {
        allOf {
            branch 'main'                                          // Only on main
            not { changeRequest() }                                // Not a PR
            expression { currentBuild.result == null }             // No prior failures
        }
    }
    steps {
        sh './scripts/deploy.sh production'
    }
}
```

### Available Conditions

| Condition | Usage | Meaning |
|-----------|-------|---------|
| `branch 'main'` | `when { branch 'main' }` | Current branch is `main` |
| `branch pattern: 'feature/*'` | `when { branch pattern: 'feature/*', comparator: 'GLOB' }` | Branch matches glob |
| `changeRequest()` | `when { changeRequest() }` | Build is for a pull request |
| `changeRequest target: 'main'` | `when { changeRequest target: 'main' }` | PR targets `main` branch |
| `environment name: 'X', value: 'Y'` | `when { environment name: 'DEPLOY_ENV', value: 'prod' }` | Env var matches value |
| `expression { ... }` | `when { expression { return params.RUN_TESTS } }` | Groovy expression is truthy |
| `tag 'v*'` | `when { tag 'v*' }` | Build is for a Git tag matching pattern |
| `allOf { ... }` | `when { allOf { branch 'main'; expression { ... } } }` | All conditions are true |
| `anyOf { ... }` | `when { anyOf { branch 'main'; branch 'develop' } }` | At least one condition is true |
| `not { ... }` | `when { not { branch 'main' } }` | Condition is false |

---

## `post` -- After the Stages

The `post` section runs after all stages complete. It has multiple condition blocks:

```groovy
pipeline {
    agent { docker { image 'golang:1.24-alpine' } }
    stages { ... }

    post {
        always {
            // Runs regardless of build result
            echo 'Pipeline finished.'

            // Archive test results even if tests failed
            junit allowEmptyResults: true, testResults: 'test-results/*.xml'

            // Archive coverage report
            archiveArtifacts artifacts: 'coverage.html', allowEmptyArchive: true

            // Clean up Docker resources
            sh 'docker system prune -f || true'
        }

        success {
            // Only runs if the build succeeded
            echo 'Build succeeded!'

            // Update GitHub commit status
            githubSetCommitStatus(
                context: 'ci/jenkins',
                state: 'SUCCESS',
                description: 'All checks passed'
            )

            // Send Slack notification
            slackSend(
                channel: '#mid-llm-cli',
                color: 'good',
                message: "Build #${BUILD_NUMBER} succeeded for ${IMAGE_TAG}"
            )
        }

        failure {
            // Only runs if the build failed
            echo 'Build failed!'

            // Update GitHub commit status
            githubSetCommitStatus(
                context: 'ci/jenkins',
                state: 'FAILURE',
                description: 'Build failed'
            )

            // Notify the team
            slackSend(
                channel: '#mid-llm-cli',
                color: 'danger',
                message: "Build #${BUILD_NUMBER} FAILED for ${IMAGE_TAG}. <${BUILD_URL}|View logs>"
            )

            // Send email to the committer
            emailext(
                subject: "FAILED: mid-llm-cli build #${BUILD_NUMBER}",
                body: "Check console output at ${BUILD_URL}",
                recipientProviders: [culprits(), requestor()]
            )
        }

        unstable {
            // Runs when the build is marked unstable (e.g., some tests failed)
            echo 'Build is unstable. Check test results.'
        }

        cleanup {
            // Always runs LAST, even after other post conditions
            // Use for guaranteed cleanup
            cleanWs()    // Delete the workspace
        }
    }
}
```

### Post Condition Order

```
Stages complete
    |
    v
post { always { ... } }       <-- Runs first, regardless of result
    |
    v
post { success/failure/unstable { ... } }   <-- Runs based on result
    |
    v
post { cleanup { ... } }      <-- Runs last, guaranteed
```

---

## Shared Libraries

When multiple Jenkins pipelines share common logic, you extract it into a **Shared Library** — a separate Git repository containing reusable Groovy functions.

### When to Use Them

- Multiple projects use the same build steps (Go build, Docker push, Slack notification)
- You want to enforce organizational standards across all pipelines
- Pipeline logic is complex enough to warrant unit testing

### Structure of a Shared Library

```
jenkins-shared-library/
  vars/
    buildGoProject.groovy       # Global function: buildGoProject()
    dockerBuildAndPush.groovy   # Global function: dockerBuildAndPush()
    notifySlack.groovy          # Global function: notifySlack()
  src/
    com/example/ci/
      GoBuilder.groovy          # Class-based helper (optional)
  resources/
    templates/
      email-template.html       # Static resources
```

### Example: `vars/buildGoProject.groovy`

```groovy
// vars/buildGoProject.groovy
def call(Map config = [:]) {
    def goVersion = config.goVersion ?: '1.24'
    def binaryName = config.binaryName ?: 'app'
    def mainPackage = config.mainPackage ?: './cmd/server'

    sh """
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -ldflags '-s -w' -o bin/${binaryName} ${mainPackage}
    """
}
```

### Using the Library in a Jenkinsfile

```groovy
@Library('jenkins-shared-library') _   // Load the library

pipeline {
    agent { docker { image 'golang:1.24-alpine' } }
    stages {
        stage('Build Server') {
            steps {
                buildGoProject(binaryName: 'claude-terminal-service', mainPackage: './cmd/server')
            }
        }
        stage('Build Poller') {
            steps {
                buildGoProject(binaryName: 'ecc-poller', mainPackage: './cmd/ecc-poller')
            }
        }
    }
}
```

### Configuring the Library in Jenkins

1. Go to **Manage Jenkins** > **Configure System** > **Global Pipeline Libraries**
2. Add a library:
   - Name: `jenkins-shared-library`
   - Default version: `main`
   - Source: Git repository URL
3. The `@Library` annotation in the Jenkinsfile references this configuration

---

## Matrix Builds

Matrix builds run the same pipeline across multiple configurations — useful for testing across Go versions, OS platforms, or database versions.

### Testing Across Go Versions

```groovy
pipeline {
    agent none

    stages {
        stage('Test Matrix') {
            matrix {
                axes {
                    axis {
                        name 'GO_VERSION'
                        values '1.23', '1.24'
                    }
                }
                stages {
                    stage('Test') {
                        agent {
                            docker { image "golang:${GO_VERSION}-alpine" }
                        }
                        steps {
                            sh 'go version'
                            sh 'go mod download'
                            sh 'go test -v ./...'
                        }
                    }
                }
            }
        }
    }
}
```

This creates two parallel builds:
```
Test Matrix
  +-- Test (Go 1.23) --> go test ./...
  +-- Test (Go 1.24) --> go test ./...
```

### Multi-Axis Matrix

```groovy
matrix {
    axes {
        axis {
            name 'GO_VERSION'
            values '1.23', '1.24'
        }
        axis {
            name 'DB_VERSION'
            values '14', '15', '16'
        }
    }
    excludes {
        // Skip Go 1.23 + Postgres 16 (known incompatibility)
        exclude {
            axis { name 'GO_VERSION'; values '1.23' }
            axis { name 'DB_VERSION'; values '16' }
        }
    }
    stages { ... }
}
```

This produces 5 combinations (2 x 3 - 1 exclusion):
```
Go 1.23 + PG 14
Go 1.23 + PG 15
Go 1.24 + PG 14
Go 1.24 + PG 15
Go 1.24 + PG 16
```

---

## `stash` / `unstash` -- Passing Files Between Stages

When stages run on different agents, the workspace is not shared. Use `stash` to save files and `unstash` to retrieve them.

### Example: Build on One Agent, Docker Build on Another

```groovy
pipeline {
    agent none

    stages {
        stage('Build Binaries') {
            agent { docker { image 'golang:1.24-alpine' } }
            steps {
                sh 'make build'
                // Save the built binaries
                stash includes: 'bin/**', name: 'binaries'
                // Save coverage report
                stash includes: 'coverage.*', name: 'coverage'
            }
        }

        stage('Docker Build') {
            agent { label 'docker-host' }
            steps {
                // Retrieve the binaries built in the previous stage
                unstash 'binaries'
                sh 'docker build -t claude-terminal-service:${BUILD_TAG} .'
            }
        }

        stage('Archive') {
            agent any
            steps {
                unstash 'binaries'
                unstash 'coverage'
                archiveArtifacts artifacts: 'bin/*, coverage.html'
            }
        }
    }
}
```

### How Stash Works

```
Agent A (golang container)         Controller              Agent B (docker host)
  |                                    |                        |
  |-- stash 'binaries' ------------->  |                        |
  |   (uploads bin/* to controller)    |                        |
  |                                    |                        |
  |                                    | <-- unstash 'binaries' |
  |                                    |   (downloads to Agent B)|
```

**Limitations:**
- Stash is stored on the Jenkins controller's disk
- Maximum size: ~100MB (configurable, but large stashes are slow)
- For large artifacts (Docker images), use a registry instead

---

## Complete Jenkinsfile for This Project

Here is a comprehensive Jenkinsfile that brings together all the concepts from this chapter:

```groovy
pipeline {
    agent {
        docker {
            image 'golang:1.24-alpine'
            args '-v go-mod-cache:/go/pkg/mod'    // Persist module cache
        }
    }

    environment {
        CGO_ENABLED   = '0'
        GOOS          = 'linux'
        GOARCH        = 'amd64'
        APP_NAME      = 'claude-terminal-service'
        IMAGE_TAG     = "${GIT_COMMIT?.take(7) ?: 'latest'}"
        REGISTRY      = credentials('docker-registry-url')
        DOCKER_CREDS  = credentials('docker-registry-credentials')
    }

    options {
        timeout(time: 30, unit: 'MINUTES')
        buildDiscarder(logRotator(numToKeepStr: '20'))
        timestamps()
        disableConcurrentBuilds()
    }

    parameters {
        choice(name: 'DEPLOY_ENV', choices: ['none', 'staging', 'production'], description: 'Deploy target')
        booleanParam(name: 'SKIP_TESTS', defaultValue: false, description: 'Skip tests (emergency only)')
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Dependencies') {
            steps {
                sh 'go mod download'
            }
        }

        stage('Quality Checks') {
            parallel {
                stage('Lint') {
                    steps {
                        sh '''
                            wget -qO- https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b /usr/local/bin
                            golangci-lint run ./...
                        '''
                    }
                }
                stage('Format') {
                    steps {
                        sh 'test -z "$(gofmt -l .)"'
                    }
                }
                stage('Vet') {
                    steps {
                        sh 'go vet ./...'
                    }
                }
            }
        }

        stage('Unit Tests') {
            when {
                expression { return !params.SKIP_TESTS }
            }
            steps {
                sh 'go test -v -race -coverprofile=coverage.out ./...'
                sh 'go tool cover -func=coverage.out'
                sh 'go tool cover -html=coverage.out -o coverage.html'
            }
            post {
                always {
                    archiveArtifacts artifacts: 'coverage.html', allowEmptyArchive: true
                }
            }
        }

        stage('Build') {
            steps {
                sh 'go build -ldflags "-s -w" -o bin/claude-terminal-service ./cmd/server'
                sh 'go build -ldflags "-s -w" -o bin/ecc-poller ./cmd/ecc-poller'
                stash includes: 'bin/**', name: 'binaries'
            }
        }

        stage('Docker Build') {
            agent { label 'docker-host' }
            steps {
                unstash 'binaries'
                sh "docker build -t ${REGISTRY}/${APP_NAME}:${IMAGE_TAG} ."
            }
        }

        stage('Deploy Staging') {
            when {
                allOf {
                    branch 'main'
                    expression { params.DEPLOY_ENV == 'staging' || params.DEPLOY_ENV == 'production' }
                }
            }
            steps {
                sh './scripts/deploy.sh staging'
            }
        }

        stage('Deploy Production') {
            when {
                allOf {
                    branch 'main'
                    expression { params.DEPLOY_ENV == 'production' }
                }
            }
            input {
                message "Deploy ${IMAGE_TAG} to production?"
                ok "Deploy"
                submitter "admin,deploy-team"
            }
            steps {
                sh './scripts/deploy.sh production'
            }
        }
    }

    post {
        always {
            archiveArtifacts artifacts: 'bin/*', allowEmptyArchive: true
        }
        failure {
            slackSend(
                channel: '#mid-llm-cli-ci',
                color: 'danger',
                message: "Build FAILED: ${env.JOB_NAME} #${env.BUILD_NUMBER} (<${env.BUILD_URL}|Open>)"
            )
        }
        cleanup {
            cleanWs()
        }
    }
}
```

---

## Summary

The Declarative Pipeline syntax provides a structured, readable way to define CI/CD pipelines. Its directives (`agent`, `environment`, `options`, `parameters`, `stages`, `post`) map directly to the pipeline design from Chapter 2. For the Claude Terminal MID Service, the key patterns are:

- Docker agent with `golang:1.24-alpine` for consistent builds
- `credentials()` for secrets management (never hardcoded)
- Parallel stages for lint, format, and vet
- `when` conditions for branch-specific deployment
- `input` for production deployment approval
- `stash`/`unstash` for passing binaries between build and Docker stages
- `post` for cleanup, notifications, and artifact archival

---

**Previous:** [Chapter 6: Docker Build & Push](06-docker-build-and-push.md)
**Next:** [Chapter 8: Advanced Jenkins Patterns](08-advanced-jenkins-patterns.md)
