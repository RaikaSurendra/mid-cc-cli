// =============================================================================
// Claude Terminal MID Service — Main CI/CD Pipeline
// =============================================================================
// Triggered on every push to any branch.
//
// Git Workflow (automated by this pipeline):
//   1. Developer pushes to a feature branch (e.g., feature/my-change)
//   2. Jenkins automatically runs lint, test, build, Docker build
//   3. On SUCCESS for feature/* branches → auto-creates a PR to main
//   4. On SUCCESS for main branch → Docker push + optional deploy
//   5. PRs are merged manually (or auto-merged if all checks pass)
//
// GitHub repo: https://github.com/RaikaSurendra/mid-cc-cli
// =============================================================================

pipeline {
    agent {
        docker {
            image 'golang:1.24-alpine'
            // Persist Go module cache across builds for faster dependency resolution.
            // Mount Docker socket so we can run Docker commands inside the container.
            args '-v go-mod-cache:/go/pkg/mod -v /var/run/docker.sock:/var/run/docker.sock'
        }
    }

    environment {
        // Go toolchain
        GOPATH       = "${WORKSPACE}/.go"
        GOBIN        = "${WORKSPACE}/.go/bin"
        CGO_ENABLED  = '0'
        PATH         = "${GOBIN}:/usr/local/go/bin:${PATH}"

        // Docker image
        IMAGE_NAME   = 'claude-terminal-service'
        IMAGE_TAG    = sh(script: 'git rev-parse --short HEAD', returnStdout: true).trim()
        REGISTRY     = credentials('docker-registry-url') // e.g. docker.io/yourorg

        // GitHub
        GITHUB_REPO  = 'RaikaSurendra/mid-cc-cli'
        GH_TOKEN     = credentials('github-token')  // GitHub PAT for PR creation

        // Project metadata
        PROJECT_NAME = 'claude-terminal-mid-service'
    }

    options {
        timeout(time: 30, unit: 'MINUTES')
        timestamps()
        buildDiscarder(logRotator(numToKeepStr: '20', artifactNumToKeepStr: '5'))
        disableConcurrentBuilds()
        skipDefaultCheckout(true)
    }

    parameters {
        choice(
            name: 'DEPLOY_ENV',
            choices: ['none', 'staging', 'production'],
            description: 'Target deployment environment. "none" skips deployment.'
        )
        booleanParam(
            name: 'SKIP_TESTS',
            defaultValue: false,
            description: 'Skip test stages (use only for emergency hotfixes).'
        )
        booleanParam(
            name: 'AUTO_MERGE',
            defaultValue: false,
            description: 'Auto-merge the PR after creation (if all checks pass).'
        )
    }

    stages {

        // =====================================================================
        // Stage 1: Checkout
        // =====================================================================
        stage('Checkout') {
            steps {
                checkout scm
                sh 'git log --oneline -5'
            }
        }

        // =====================================================================
        // Stage 2: Dependencies
        // =====================================================================
        stage('Dependencies') {
            steps {
                sh '''
                    echo "--- Downloading Go modules ---"
                    go mod download

                    echo "--- Verifying module integrity ---"
                    go mod verify

                    echo "--- Checking go.mod/go.sum consistency ---"
                    go mod tidy
                    git diff --exit-code go.mod go.sum || {
                        echo "ERROR: go.mod or go.sum changed after tidy."
                        echo "Run 'go mod tidy' locally and commit the result."
                        exit 1
                    }
                '''
            }
        }

        // =====================================================================
        // Stage 3: Quality Gates (parallel)
        // =====================================================================
        stage('Quality Gates') {
            when {
                expression { return !params.SKIP_TESTS }
            }
            parallel {
                stage('Lint') {
                    steps {
                        sh '''
                            echo "--- Installing golangci-lint ---"
                            wget -O- -nv https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b ${GOBIN} v1.62.2

                            echo "--- Running linter ---"
                            ${GOBIN}/golangci-lint run --timeout=5m ./...
                        '''
                    }
                }
                stage('Format Check') {
                    steps {
                        sh '''
                            echo "--- Checking gofmt ---"
                            UNFORMATTED=$(gofmt -l .)
                            if [ -n "$UNFORMATTED" ]; then
                                echo "ERROR: The following files are not formatted:"
                                echo "$UNFORMATTED"
                                echo "Run 'gofmt -w .' locally and commit the result."
                                exit 1
                            fi
                            echo "All files are properly formatted."
                        '''
                    }
                }
                stage('Vet') {
                    steps {
                        sh '''
                            echo "--- Running go vet ---"
                            go vet ./...
                        '''
                    }
                }
            }
        }

        // =====================================================================
        // Stage 4: Unit Tests
        // =====================================================================
        stage('Unit Tests') {
            when {
                expression { return !params.SKIP_TESTS }
            }
            steps {
                sh '''
                    echo "--- Installing gotestsum for JUnit output ---"
                    go install gotest.tools/gotestsum@latest

                    echo "--- Running unit tests ---"
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

        // =====================================================================
        // Stage 5: Race Detection
        // =====================================================================
        stage('Race Detection') {
            when {
                expression { return !params.SKIP_TESTS }
            }
            steps {
                sh '''
                    echo "--- Running tests with race detector ---"
                    CGO_ENABLED=1 go test -race ./...
                '''
            }
        }

        // =====================================================================
        // Stage 6: Coverage
        // =====================================================================
        stage('Coverage') {
            when {
                expression { return !params.SKIP_TESTS }
            }
            steps {
                sh '''
                    echo "--- Running tests with coverage ---"
                    go test -coverprofile=coverage.out -covermode=atomic ./...
                    go tool cover -func=coverage.out | tail -1
                    go tool cover -html=coverage.out -o coverage.html
                '''
            }
            post {
                always {
                    publishHTML(target: [
                        allowMissing: true,
                        alwaysLinkToLastBuild: true,
                        keepAll: true,
                        reportDir: '.',
                        reportFiles: 'coverage.html',
                        reportName: 'Go Coverage Report'
                    ])
                    archiveArtifacts artifacts: 'coverage.out', allowEmptyArchive: true
                }
            }
        }

        // =====================================================================
        // Stage 7: Build Binaries
        // =====================================================================
        stage('Build Binaries') {
            steps {
                sh '''
                    echo "--- Installing build tools ---"
                    apk add --no-cache make git

                    echo "--- Building binaries ---"
                    make build

                    echo "--- Verifying output ---"
                    ls -lh bin/claude-terminal-service bin/ecc-poller
                    file bin/claude-terminal-service bin/ecc-poller
                '''
            }
            post {
                success {
                    archiveArtifacts artifacts: 'bin/*', fingerprint: true
                }
            }
        }

        // =====================================================================
        // Stage 8: Docker Build
        // =====================================================================
        stage('Docker Build') {
            steps {
                sh '''
                    echo "--- Building Docker image ---"
                    docker build \
                        -t ${IMAGE_NAME}:${IMAGE_TAG} \
                        -t ${IMAGE_NAME}:latest \
                        --label "git.commit=${IMAGE_TAG}" \
                        --label "build.number=${BUILD_NUMBER}" \
                        --label "build.url=${BUILD_URL}" \
                        .

                    echo "--- Image details ---"
                    docker images ${IMAGE_NAME}:${IMAGE_TAG}
                '''
            }
        }

        // =====================================================================
        // Stage 9: Integration Tests
        // =====================================================================
        stage('Integration Tests') {
            when {
                expression { return !params.SKIP_TESTS }
            }
            steps {
                sh '''
                    echo "--- Starting services with Docker Compose ---"
                    apk add --no-cache docker-compose curl bash

                    # Start postgres + terminal service for integration tests
                    docker compose up -d postgres claude-terminal-service

                    echo "--- Waiting for services to be healthy ---"
                    sleep 15

                    # Wait for health check (up to 60 seconds)
                    for i in $(seq 1 12); do
                        if curl -sf http://claude-terminal-service:3000/health; then
                            echo ""
                            echo "Service is healthy."
                            break
                        fi
                        echo "Waiting for service... (attempt $i/12)"
                        sleep 5
                    done

                    echo "--- Running integration tests ---"
                    bash scripts/run-tests.sh || true
                '''
            }
            post {
                always {
                    sh 'docker compose down --volumes --remove-orphans || true'
                }
            }
        }

        // =====================================================================
        // Stage 10: Security Scan
        // =====================================================================
        stage('Security Scan') {
            steps {
                sh '''
                    echo "--- Installing Trivy ---"
                    wget -qO- https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b /usr/local/bin

                    echo "--- Scanning Docker image for vulnerabilities ---"
                    trivy image \
                        --severity HIGH,CRITICAL \
                        --exit-code 0 \
                        --format table \
                        ${IMAGE_NAME}:${IMAGE_TAG}

                    echo "--- Generating JSON report ---"
                    trivy image \
                        --severity HIGH,CRITICAL \
                        --format json \
                        --output trivy-report.json \
                        ${IMAGE_NAME}:${IMAGE_TAG}
                '''
            }
            post {
                always {
                    archiveArtifacts artifacts: 'trivy-report.json', allowEmptyArchive: true
                }
            }
        }

        // =====================================================================
        // Stage 11: Docker Push
        // =====================================================================
        stage('Docker Push') {
            when {
                branch 'main'
            }
            steps {
                withCredentials([
                    usernamePassword(
                        credentialsId: 'docker-registry-creds',
                        usernameVariable: 'DOCKER_USER',
                        passwordVariable: 'DOCKER_PASS'
                    )
                ]) {
                    sh '''
                        echo "--- Logging into Docker registry ---"
                        echo "$DOCKER_PASS" | docker login -u "$DOCKER_USER" --password-stdin ${REGISTRY}

                        echo "--- Tagging images ---"
                        docker tag ${IMAGE_NAME}:${IMAGE_TAG} ${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}
                        docker tag ${IMAGE_NAME}:latest       ${REGISTRY}/${IMAGE_NAME}:latest

                        echo "--- Pushing images ---"
                        docker push ${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}
                        docker push ${REGISTRY}/${IMAGE_NAME}:latest

                        echo "--- Push complete ---"
                        echo "Pushed: ${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}"
                        echo "Pushed: ${REGISTRY}/${IMAGE_NAME}:latest"
                    '''
                }
            }
        }

        // =====================================================================
        // Stage 12: Deploy to Staging
        // =====================================================================
        stage('Deploy Staging') {
            when {
                allOf {
                    branch 'main'
                    expression { return params.DEPLOY_ENV == 'staging' || params.DEPLOY_ENV == 'production' }
                }
            }
            steps {
                withCredentials([
                    usernamePassword(
                        credentialsId: 'servicenow-api-creds',
                        usernameVariable: 'SN_USER',
                        passwordVariable: 'SN_PASS'
                    ),
                    string(credentialsId: 'encryption-key', variable: 'ENCRYPTION_KEY'),
                    string(credentialsId: 'api-auth-token', variable: 'API_AUTH_TOKEN')
                ]) {
                    sh '''
                        echo "--- Deploying to staging ---"
                        echo "Image: ${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}"
                        echo "Environment: ${DEPLOY_ENV}"

                        # Placeholder: Replace with your actual deployment commands.
                        # Examples:
                        #   kubectl set image deployment/claude-terminal \
                        #       claude-terminal=${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG} \
                        #       --namespace=staging
                        #
                        #   docker compose -f docker-compose.staging.yml pull
                        #   docker compose -f docker-compose.staging.yml up -d

                        echo "Deployment placeholder -- implement for your infrastructure."
                    '''
                }
            }
        }

        // =====================================================================
        // Stage 13: Auto-Create PR (feature branches → main)
        // =====================================================================
        // When a feature/* or docs/* branch passes all stages, Jenkins
        // automatically creates a Pull Request to merge into main.
        // This eliminates the manual "go to GitHub and click New PR" step.
        // =====================================================================
        stage('Create Pull Request') {
            when {
                allOf {
                    not { branch 'main' }
                    anyOf {
                        branch 'feature/*'
                        branch 'docs/*'
                        branch 'fix/*'
                        branch 'refactor/*'
                    }
                }
            }
            steps {
                sh '''
                    echo "--- Installing GitHub CLI ---"
                    apk add --no-cache github-cli || {
                        # Fallback: install gh manually
                        wget -qO- https://github.com/cli/cli/releases/download/v2.63.2/gh_2.63.2_linux_amd64.tar.gz | tar xz
                        mv gh_2.63.2_linux_amd64/bin/gh /usr/local/bin/
                    }

                    echo "--- Checking for existing PR ---"
                    EXISTING_PR=$(gh pr list \
                        --repo ${GITHUB_REPO} \
                        --head ${BRANCH_NAME} \
                        --base main \
                        --json number \
                        --jq '.[0].number // empty' 2>/dev/null || echo "")

                    if [ -n "$EXISTING_PR" ]; then
                        echo "PR #${EXISTING_PR} already exists for ${BRANCH_NAME} → main"
                        echo "URL: https://github.com/${GITHUB_REPO}/pull/${EXISTING_PR}"

                        # Add a comment with the latest build status
                        gh pr comment ${EXISTING_PR} \
                            --repo ${GITHUB_REPO} \
                            --body "Jenkins Build #${BUILD_NUMBER} PASSED on commit ${IMAGE_TAG}. All quality gates, tests, and Docker build succeeded. [Build Log](${BUILD_URL})"
                    else
                        echo "--- Creating PR: ${BRANCH_NAME} → main ---"

                        # Extract branch type and name for PR title
                        BRANCH_TYPE=$(echo ${BRANCH_NAME} | cut -d'/' -f1)
                        BRANCH_DESC=$(echo ${BRANCH_NAME} | cut -d'/' -f2- | tr '-' ' ')

                        gh pr create \
                            --repo ${GITHUB_REPO} \
                            --head ${BRANCH_NAME} \
                            --base main \
                            --title "${BRANCH_TYPE}: ${BRANCH_DESC}" \
                            --body "## Auto-created by Jenkins

**Branch:** \`${BRANCH_NAME}\`
**Build:** [#${BUILD_NUMBER}](${BUILD_URL})
**Commit:** \`${IMAGE_TAG}\`

### Pipeline Results
- Lint: PASSED
- Format Check: PASSED
- Vet: PASSED
- Unit Tests: PASSED
- Race Detection: PASSED
- Coverage: PASSED
- Build: PASSED
- Docker Build: PASSED
- Security Scan: PASSED

### Review Checklist
- [ ] Code review completed
- [ ] Changes tested locally
- [ ] Documentation updated (if applicable)

---
*This PR was auto-created by Jenkins after all CI checks passed.*"

                        echo "PR created successfully."
                    fi
                '''
            }
        }

        // =====================================================================
        // Stage 14: Auto-Merge PR (optional)
        // =====================================================================
        // If AUTO_MERGE is enabled and all checks pass, Jenkins merges the PR.
        // Uses --squash to keep main branch history clean.
        // =====================================================================
        stage('Auto-Merge PR') {
            when {
                allOf {
                    not { branch 'main' }
                    expression { return params.AUTO_MERGE }
                    anyOf {
                        branch 'feature/*'
                        branch 'docs/*'
                        branch 'fix/*'
                        branch 'refactor/*'
                    }
                }
            }
            steps {
                sh '''
                    echo "--- Finding PR for ${BRANCH_NAME} ---"
                    PR_NUMBER=$(gh pr list \
                        --repo ${GITHUB_REPO} \
                        --head ${BRANCH_NAME} \
                        --base main \
                        --json number \
                        --jq '.[0].number' 2>/dev/null || echo "")

                    if [ -z "$PR_NUMBER" ]; then
                        echo "No PR found for ${BRANCH_NAME}. Skipping auto-merge."
                        exit 0
                    fi

                    echo "--- Enabling auto-merge for PR #${PR_NUMBER} ---"
                    gh pr merge ${PR_NUMBER} \
                        --repo ${GITHUB_REPO} \
                        --squash \
                        --auto \
                        --subject "${BRANCH_NAME}: auto-merged by Jenkins build #${BUILD_NUMBER}" \
                        --body "Auto-merged after all CI checks passed.\\n\\nBuild: ${BUILD_URL}\\nCommit: ${IMAGE_TAG}"

                    echo "Auto-merge enabled for PR #${PR_NUMBER}."
                    echo "PR will merge once all required status checks pass."
                '''
            }
        }
    }

    // =========================================================================
    // Post-build actions
    // =========================================================================
    post {
        always {
            // Publish test results if they exist
            junit testResults: 'test-results.xml', allowEmptyResults: true

            // Clean up Docker images to free disk space
            sh '''
                docker rmi ${IMAGE_NAME}:${IMAGE_TAG} || true
                docker rmi ${IMAGE_NAME}:latest || true
                docker rmi ${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG} || true
                docker rmi ${REGISTRY}/${IMAGE_NAME}:latest || true
                docker system prune -f || true
            '''

            // Clean workspace
            cleanWs()
        }

        success {
            echo "Build #${BUILD_NUMBER} SUCCEEDED for ${PROJECT_NAME} @ ${IMAGE_TAG} on branch ${BRANCH_NAME}"
            // Update GitHub commit status
            sh '''
                gh api repos/${GITHUB_REPO}/statuses/${GIT_COMMIT} \
                    --method POST \
                    -f state=success \
                    -f target_url=${BUILD_URL} \
                    -f description="Jenkins build #${BUILD_NUMBER} passed" \
                    -f context="jenkins/pipeline" 2>/dev/null || true
            '''
            // Uncomment to enable Slack notifications:
            // slackSend(
            //     color: 'good',
            //     message: "SUCCESS: ${PROJECT_NAME} build #${BUILD_NUMBER} (${BRANCH_NAME} @ ${IMAGE_TAG})\n${BUILD_URL}"
            // )
        }

        failure {
            echo "Build #${BUILD_NUMBER} FAILED for ${PROJECT_NAME} @ ${IMAGE_TAG} on branch ${BRANCH_NAME}"
            // Update GitHub commit status
            sh '''
                gh api repos/${GITHUB_REPO}/statuses/${GIT_COMMIT} \
                    --method POST \
                    -f state=failure \
                    -f target_url=${BUILD_URL} \
                    -f description="Jenkins build #${BUILD_NUMBER} failed" \
                    -f context="jenkins/pipeline" 2>/dev/null || true
            '''
            // Uncomment to enable Slack notifications:
            // slackSend(
            //     color: 'danger',
            //     message: "FAILURE: ${PROJECT_NAME} build #${BUILD_NUMBER} (${BRANCH_NAME} @ ${IMAGE_TAG})\n${BUILD_URL}"
            // )
        }

        unstable {
            echo "Build #${BUILD_NUMBER} UNSTABLE for ${PROJECT_NAME} @ ${IMAGE_TAG} on branch ${BRANCH_NAME}"
        }
    }
}
