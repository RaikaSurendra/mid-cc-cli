# Chapter 9: Jenkins Best Practices and Pitfalls

## Top 10 Jenkins Mistakes Beginners Make

### Mistake #1: Running Builds on the Controller

**The problem:** The Jenkins controller (master) manages the UI, schedules builds, stores credentials, and serves the API. Running builds on it competes for resources, causes instability, and creates a security risk — a malicious build script could access Jenkins secrets or modify the controller.

**What it looks like:**
```groovy
// BAD: runs on the controller
pipeline {
    agent any   // If no agents are configured, "any" defaults to the controller
    stages {
        stage('Build') {
            steps { sh 'make build' }
        }
    }
}
```

**The fix:** Always use dedicated agents. Configure the controller to have **zero executors** (Manage Jenkins > Configure System > # of executors = 0).

```groovy
// GOOD: runs on a dedicated agent
pipeline {
    agent {
        docker { image 'golang:1.24-alpine' }
    }
    stages {
        stage('Build') {
            steps { sh 'make build' }
        }
    }
}
```

**For this project:** Use a Docker agent with `golang:1.24-alpine` for Go builds and a `docker-host` labeled agent for stages that need Docker daemon access.

---

### Mistake #2: Storing Secrets in the Jenkinsfile

**The problem:** Jenkinsfiles are committed to Git. Secrets in the Jenkinsfile are visible to anyone with read access to the repository. Even if you delete the line later, the secret remains in Git history forever.

**What it looks like:**
```groovy
// BAD: credentials in plain text
environment {
    API_TOKEN = 'sk-ant-abc123-real-secret-token'
    DB_PASSWORD = 'production_password_2024'
}
```

**The fix:** Store secrets in Jenkins Credential Store and reference them with `credentials()`.

```groovy
// GOOD: secrets from Jenkins credential store
environment {
    API_TOKEN   = credentials('anthropic-api-token')        // Secret text
    DB_CREDS    = credentials('postgresql-credentials')      // Username + password
    DOCKER_CREDS = credentials('docker-registry-credentials') // Username + password
}
```

**For this project:** The following secrets must be stored in Jenkins, never in the Jenkinsfile:
| Secret | Credential Type | Usage |
|--------|----------------|-------|
| `API_AUTH_TOKEN` | Secret text | Bearer token for the HTTP service |
| `ENCRYPTION_KEY` | Secret text | AES-256-GCM key for credential encryption |
| `SERVICENOW_API_PASSWORD` | Username with password | ServiceNow API access |
| `DB_PASSWORD` | Secret text | PostgreSQL database password |
| Docker registry credentials | Username with password | Docker push |

---

### Mistake #3: Not Cleaning Up Docker Images

**The problem:** Every Docker build creates layers, images, and build cache on the Jenkins agent. Without cleanup, the disk fills up. A single `docker build` can consume 500MB-1GB. After 50 builds, that is 25-50GB of unused images.

**What it looks like:**
```bash
$ docker system df
TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE
Images          147       3         48.2GB    45.1GB (93%)
Containers      5         5         102MB     0B (0%)
Build Cache     89        0         12.3GB    12.3GB
```

**The fix:** Add cleanup to the `post` section of every pipeline.

```groovy
post {
    cleanup {
        // Remove the image built during this pipeline
        sh "docker rmi ${REGISTRY}/${APP_NAME}:${IMAGE_TAG} || true"

        // Prune dangling images and build cache
        sh 'docker system prune -f --filter "until=24h"'

        // Clean the Jenkins workspace
        cleanWs()
    }
}
```

**Scheduled cleanup:** Add a nightly maintenance job that runs aggressive cleanup:

```groovy
// Jenkinsfile.cleanup
pipeline {
    agent { label 'docker-host' }
    triggers { cron('H 3 * * *') }    // 3 AM nightly
    stages {
        stage('Docker Cleanup') {
            steps {
                sh 'docker system prune -af --filter "until=48h"'    // Remove ALL unused images older than 48h
                sh 'docker volume prune -f'                          // Remove unused volumes
            }
        }
    }
}
```

---

### Mistake #4: Giant Monolithic Jenkinsfile

**The problem:** A Jenkinsfile that grows to 500+ lines becomes hard to read, review, and debug. Changes to one stage risk breaking another. Multiple teams editing the same file causes merge conflicts.

**What it looks like:**
```groovy
// BAD: 600-line Jenkinsfile with everything inline
pipeline {
    stages {
        stage('Lint') {
            steps {
                // 50 lines of golangci-lint setup and custom rules
            }
        }
        stage('Test') {
            steps {
                // 80 lines of test setup, execution, and coverage parsing
            }
        }
        stage('Docker') {
            steps {
                // 100 lines of Docker build, tag, push logic
            }
        }
        stage('Deploy') {
            steps {
                // 150 lines of deployment scripts for 3 environments
            }
        }
        // ... continues for hundreds more lines
    }
}
```

**The fix:** Use a combination of:

1. **External scripts:** Move complex logic into shell scripts in the repository.

```groovy
// GOOD: Jenkinsfile stays lean
stage('Deploy Staging') {
    steps {
        sh './scripts/deploy.sh staging'
    }
}
```

2. **Shared Libraries:** Extract reusable patterns into a shared library (see Chapter 7).

```groovy
// GOOD: one-line stage using shared library
stage('Build') {
    steps {
        buildGoProject(binaryName: 'claude-terminal-service', mainPackage: './cmd/server')
    }
}
```

3. **Keep the Jenkinsfile under 200 lines.** It should be an orchestration layer — calling scripts and libraries, not containing business logic.

---

### Mistake #5: No Build Timeout

**The problem:** A stuck build (waiting for a network response, a deadlocked test, or an unresponsive Docker daemon) runs forever, occupying an executor and blocking the build queue.

**What it looks like:**
```
Build #42 - Started 14 hours ago - Still running
Build #43 - Waiting in queue (no available executors)
Build #44 - Waiting in queue
Build #45 - Waiting in queue
```

**The fix:** Set timeouts at the pipeline level and per-stage.

```groovy
pipeline {
    options {
        timeout(time: 30, unit: 'MINUTES')    // Entire pipeline
    }

    stages {
        stage('Integration Tests') {
            options {
                timeout(time: 10, unit: 'MINUTES')    // Just this stage
            }
            steps {
                sh 'go test -v -tags=integration ./...'
            }
        }

        stage('Deploy Production') {
            options {
                timeout(time: 5, unit: 'MINUTES')     // Don't wait forever for approval
            }
            input {
                message "Deploy to production?"
            }
            steps {
                sh './scripts/deploy.sh production'
            }
        }
    }
}
```

**For this project:** The Docker build stage (which installs Claude CLI via npm) can hang if the npm registry is unreachable. A 10-minute timeout on that stage prevents indefinite blocking.

---

### Mistake #6: Not Archiving Test Results

**The problem:** Tests fail, but the console output scrolls past thousands of lines. Without structured test results, developers spend 20 minutes searching logs for the one failing test assertion.

**What it looks like:**
```
Build #42 FAILED
Console output: 15,847 lines
Ctrl+F "FAIL" ... 47 matches (most are "FAILOVER", "DEFAULT_FAIL_HANDLER", etc.)
```

**The fix:** Generate JUnit XML reports and publish them with the `junit` step.

```groovy
stage('Unit Tests') {
    steps {
        // Use gotestsum for JUnit output
        sh '''
            go install gotest.tools/gotestsum@latest
            gotestsum --junitfile test-results/unit-tests.xml -- -v -race -coverprofile=coverage.out ./...
        '''
    }
    post {
        always {
            // Jenkins parses XML and shows a test results summary
            junit 'test-results/*.xml'

            // Also archive the coverage report
            archiveArtifacts artifacts: 'coverage.html', allowEmptyArchive: true
        }
    }
}
```

Jenkins then shows:

```
Build #42 FAILED
Test Result: 45 passed, 2 failed, 0 skipped

Failed Tests:
  TestSessionManager_CreateSession/duplicate_user_id
    session_test.go:78: expected error "session exists", got nil

  TestRateLimiter_BurstExceeded
    ratelimit_test.go:112: expected 429 status, got 200
```

Each failed test is clickable, showing the assertion, stack trace, and duration.

---

### Mistake #7: Ignoring Flaky Tests

**The problem:** A test passes 90% of the time but fails randomly due to timing, network, or race conditions. Developers start re-running the pipeline ("try again, it usually works"), which erodes trust in the CI system. Eventually, failures are ignored entirely.

**What it looks like:**
```
Build #40 - PASS
Build #41 - FAIL (TestSessionTimeout - timing issue)     <-- "just re-run it"
Build #42 - PASS                                          <-- "see, it's fine"
Build #43 - FAIL (TestSessionTimeout - timing issue)     <-- *sigh*
Build #44 - PASS
```

**The fix:**

1. **Identify flaky tests.** Jenkins Test Results Analyzer plugin tracks test pass/fail history over time.

2. **Fix the root cause.** In this project, common flakiness sources:
   - **Timing:** `time.Sleep(100 * time.Millisecond)` in tests is unreliable. Use channels or polling with timeouts instead.
   - **Port conflicts:** Integration tests bind to port 3000, conflicting with parallel runs. Use dynamic port allocation.
   - **Race conditions:** The session manager uses goroutines. Run tests with `-race` to detect data races.

3. **Quarantine flaky tests.** If you cannot fix a test immediately, mark it and track it:

```go
func TestSessionTimeout(t *testing.T) {
    if os.Getenv("SKIP_FLAKY") == "true" {
        t.Skip("Flaky test - tracked in issue #42")
    }
    // ... test code
}
```

4. **Never ignore legitimate failures.** If a test fails twice in a row, it is not flaky — something is broken.

---

### Mistake #8: No Pipeline-as-Code

**The problem:** Configuring the pipeline through the Jenkins web UI (clicking through forms, adding build steps with dropdowns) creates a fragile configuration that:
- Cannot be version-controlled
- Cannot be code-reviewed
- Cannot be replicated on another Jenkins instance
- Is lost if Jenkins is reinstalled

**What it looks like:**

Someone manually configured 15 build steps, 8 post-build actions, and 3 email notifications through the UI. Nobody remembers the exact configuration. When Jenkins is migrated, all of it is lost.

**The fix:** Use a `Jenkinsfile` in the repository. Always.

```groovy
// Jenkinsfile -- committed alongside source code
pipeline {
    agent { docker { image 'golang:1.24-alpine' } }
    stages {
        stage('Build') {
            steps { sh 'make build' }
        }
    }
}
```

Benefits:
- **Versioned:** Changes to the pipeline are tracked in Git with author, date, and context
- **Reviewable:** Pipeline changes go through pull request review like any other code
- **Reproducible:** Clone the repo, and you have the complete build configuration
- **Portable:** Move to a new Jenkins instance by pointing at the same repository

---

### Mistake #9: Over-Triggering Builds

**The problem:** Triggering a build on every single push to every branch wastes compute resources and floods the build queue.

**What it looks like:**
```
Developer pushes 5 commits in 2 minutes (fix typo, fix typo again, add comment, remove comment, actual fix)
Result: 5 builds queued, 4 of which are immediately obsolete
```

**The fix:**

1. **Quiet period:** Wait 30 seconds after a trigger before starting the build. If another push arrives during the quiet period, the timer resets. Only the latest push gets built.

```groovy
pipeline {
    options {
        quietPeriod(30)    // Wait 30 seconds for additional pushes
    }
}
```

2. **Branch filtering:** Do not build branches that are not ready for CI.

```groovy
// In multibranch configuration:
// Only build branches matching: main, develop, feature/*, hotfix/*
// Ignore: wip/*, draft/*, experiment/*
```

3. **Path filtering:** Skip builds when only documentation or non-code files change.

```groovy
stage('Build') {
    when {
        changeset '**/*.go'    // Only run if Go files changed
    }
    steps {
        sh 'make build'
    }
}
```

4. **Skip CI commit message:** Allow developers to skip CI for non-code commits.

```groovy
stage('Check Skip CI') {
    steps {
        script {
            def commitMsg = sh(script: 'git log -1 --pretty=%B', returnStdout: true).trim()
            if (commitMsg.contains('[skip ci]') || commitMsg.contains('[ci skip]')) {
                currentBuild.result = 'NOT_BUILT'
                error('Skipping build per commit message')
            }
        }
    }
}
```

---

### Mistake #10: Not Backing Up Jenkins

**The problem:** Jenkins stores configuration, job definitions, build history, credentials, and plugin data on disk. A disk failure, accidental deletion, or botched upgrade can destroy years of CI/CD configuration.

**What it looks like:**
```
$ ls /var/lib/jenkins/
ls: /var/lib/jenkins/: No such file or directory

"Does anyone remember how the deployment pipeline was configured?"
"No."
```

**The fix:**

1. **Jenkins Configuration as Code (JCasC):** Define Jenkins configuration in a YAML file, committed to Git.

```yaml
# jenkins.yaml (JCasC)
jenkins:
  systemMessage: "Claude Terminal MID Service CI"
  numExecutors: 0            # No builds on controller
  securityRealm:
    ldap:
      configurations:
        - server: "ldap.example.com"
  authorizationStrategy:
    roleBased:
      roles:
        global:
          - name: "admin"
            permissions: ["Overall/Administer"]
          - name: "developer"
            permissions: ["Job/Build", "Job/Read"]
```

2. **ThinBackup Plugin:** Automated periodic backups of Jenkins configuration.

Configuration:
- Backup directory: `/backups/jenkins/`
- Full backup schedule: Weekly (Sunday 2 AM)
- Differential backup schedule: Daily (2 AM)
- Max backup sets: 10
- Backup build results: No (too large; build history is disposable)

3. **Critical files to back up:**
   - `$JENKINS_HOME/config.xml` (global configuration)
   - `$JENKINS_HOME/credentials.xml` (encrypted credentials)
   - `$JENKINS_HOME/jobs/*/config.xml` (job configurations)
   - `$JENKINS_HOME/*.xml` (plugin configurations)
   - `$JENKINS_HOME/secrets/` (encryption keys for credentials)

4. **What NOT to back up** (too large, regeneratable):
   - `$JENKINS_HOME/jobs/*/builds/` (build history and logs)
   - `$JENKINS_HOME/jobs/*/workspace/` (temporary build files)
   - `$JENKINS_HOME/war/` (Jenkins WAR file; re-downloaded on install)

---

## Security Hardening

### Role-Based Access Control (RBAC)

**Plugin:** Role-Based Authorization Strategy

```
Roles for mid-llm-cli project:
  admin       -- Full access (manage Jenkins, configure jobs, manage credentials)
  developer   -- Build, read, cancel jobs. Cannot configure or delete.
  viewer      -- Read-only access to build results and logs.

Users:
  john.doe    -- developer
  jane.smith  -- developer
  ops-team    -- admin
  stakeholder -- viewer
```

### CSRF Protection

Jenkins enables CSRF protection by default. Never disable it. If API calls fail with "403 No valid crumb," use the crumb API:

```bash
# Get a crumb
CRUMB=$(curl -s -u user:token 'http://jenkins:8080/crumbIssuer/api/json' | jq -r '.crumb')

# Use the crumb in subsequent requests
curl -X POST -u user:token -H "Jenkins-Crumb: $CRUMB" \
    'http://jenkins:8080/job/mid-llm-cli/build'
```

### Script Approval

When a Jenkinsfile uses Groovy methods not in the approved list, Jenkins blocks execution and requires an admin to approve the script. This prevents malicious Groovy code from escaping the sandbox.

**Common approvals needed for this project:**
- `method groovy.lang.GString` -- String interpolation in `sh` steps
- `method java.lang.String toFloat` -- Parsing coverage percentage
- `staticMethod org.codehaus.groovy.runtime.DefaultGroovyMethods` -- Various Groovy helpers

Review each approval carefully. A malicious PR could sneak in `System.exit(0)` or credential-reading code.

### Credential Rotation

Rotate credentials on a schedule:

| Credential | Rotation Frequency | How to Rotate |
|-----------|-------------------|---------------|
| Jenkins admin password | Every 90 days | Manage Jenkins > Users |
| API tokens | Every 90 days | User > Configure > API Token |
| ServiceNow API password | Per org policy | Jenkins Credentials + ServiceNow admin |
| Docker registry token | Every 90 days | Jenkins Credentials + registry admin |
| Encryption key | Annually | Generate new key, re-encrypt data, update Jenkins credential |

---

## Performance Optimization

### Agent Scaling

| Load | Agents | Configuration |
|------|--------|---------------|
| Low (< 10 builds/day) | 1-2 static agents | Persistent VMs, 4 CPU, 8GB RAM each |
| Medium (10-50 builds/day) | 2-4 static agents | Persistent VMs + 2 on-demand Docker agents |
| High (50+ builds/day) | Kubernetes pod agents | Auto-scaling pods, 1 executor each, spin up/down per build |

### Pipeline Optimization Checklist

| Optimization | Savings | Difficulty |
|-------------|---------|------------|
| Cache Go modules (Docker volume) | 30-45 seconds per build | Easy |
| Cache Docker layers (`--cache-from`) | 2-4 minutes per Docker build | Easy |
| Pre-bake CI Docker image with tools | 20-30 seconds per build | Medium |
| Parallel lint/format/vet | 30-50 seconds per build | Easy |
| Parallel test packages | Depends on test suite size | Medium |
| Skip unchanged stages (path filter) | Entire stage duration | Easy |
| Quiet period for rapid pushes | Eliminates redundant builds | Easy |

### Distributed Builds

For large teams, distribute builds across a pool of agents:

```
Controller
  |
  +-- Agent Pool: "golang" (label)
  |     +-- agent-go-1 (4 CPU, 8GB, 2 executors)
  |     +-- agent-go-2 (4 CPU, 8GB, 2 executors)
  |     +-- agent-go-3 (4 CPU, 8GB, 2 executors)
  |
  +-- Agent Pool: "docker" (label)
  |     +-- agent-docker-1 (8 CPU, 16GB, Docker daemon, 1 executor)
  |     +-- agent-docker-2 (8 CPU, 16GB, Docker daemon, 1 executor)
  |
  +-- Agent Pool: "deploy" (label)
        +-- agent-deploy-1 (2 CPU, 4GB, kubectl + ssh, 1 executor)
```

```groovy
// Pipeline uses label selectors
stage('Build')  { agent { label 'golang' } }
stage('Docker') { agent { label 'docker' } }
stage('Deploy') { agent { label 'deploy' } }
```

---

## Monitoring Jenkins Itself

Jenkins is infrastructure. Like any infrastructure, it needs monitoring.

### Prometheus Metrics

**Plugin:** Prometheus Metrics Plugin

Once installed, metrics are available at `http://jenkins:8080/prometheus`.

**Key metrics to monitor:**

| Metric | Meaning | Alert Threshold |
|--------|---------|----------------|
| `jenkins_queue_size_value` | Builds waiting in queue | > 10 for > 5 minutes |
| `jenkins_node_offline_value` | Number of offline agents | > 0 |
| `jenkins_executor_in_use_value` | Busy executors | > 90% for > 15 minutes |
| `jenkins_runs_total_count` (failure) | Failed builds | > 50% failure rate |
| `jenkins_health_check_score` | Overall health (0-100) | < 80 |
| `process_resident_memory_bytes` | Jenkins memory usage | > 80% of allocated |
| `jenkins_disk_usage_bytes` | Disk usage on controller | > 80% of capacity |

### Health Checks

Jenkins exposes health checks at `http://jenkins:8080/manage`:

```bash
# Quick health check
curl -s http://jenkins:8080/api/json?tree=assignedLabels,mode,nodeDescription,quietingDown | jq .

# Queue status
curl -s http://jenkins:8080/queue/api/json | jq '.items | length'

# Agent status
curl -s http://jenkins:8080/computer/api/json | jq '.computer[] | {name: .displayName, offline: .offline}'
```

---

## Backup Strategy

### What to Back Up

```
CRITICAL (configuration):
  /var/lib/jenkins/config.xml                    # Global config
  /var/lib/jenkins/credentials.xml               # Encrypted credentials
  /var/lib/jenkins/secrets/                       # Encryption keys
  /var/lib/jenkins/users/                         # User accounts
  /var/lib/jenkins/jobs/*/config.xml             # Job definitions
  /var/lib/jenkins/nodes/*/config.xml            # Agent definitions
  /var/lib/jenkins/*.xml                          # Plugin configs

IMPORTANT (reproducibility):
  /var/lib/jenkins/plugins/*.jpi                 # Installed plugins
  /var/lib/jenkins/jenkins.yaml                  # JCasC config (if used)

SKIP (large, regeneratable):
  /var/lib/jenkins/jobs/*/builds/                # Build history (terabytes)
  /var/lib/jenkins/jobs/*/workspace/             # Temporary files
  /var/lib/jenkins/war/                          # Jenkins application
  /var/lib/jenkins/caches/                       # Download caches
```

### Automated Backup Script

```bash
#!/bin/bash
# /opt/scripts/backup-jenkins.sh

JENKINS_HOME="/var/lib/jenkins"
BACKUP_DIR="/backups/jenkins/$(date +%Y%m%d-%H%M%S)"

mkdir -p "$BACKUP_DIR"

# Back up configuration files
tar -czf "$BACKUP_DIR/jenkins-config.tar.gz" \
    -C "$JENKINS_HOME" \
    config.xml \
    credentials.xml \
    secrets/ \
    users/ \
    nodes/ \
    jenkins.yaml \
    $(find "$JENKINS_HOME/jobs" -maxdepth 2 -name config.xml -printf 'jobs/%P\n')

# Back up plugin list (for reproducibility)
ls "$JENKINS_HOME/plugins/" | grep '.jpi$' | sed 's/.jpi$//' > "$BACKUP_DIR/plugin-list.txt"

# Retain last 30 backups
ls -dt /backups/jenkins/*/ | tail -n +31 | xargs rm -rf

echo "Backup completed: $BACKUP_DIR"
```

Schedule with cron:
```
0 2 * * * /opt/scripts/backup-jenkins.sh >> /var/log/jenkins-backup.log 2>&1
```

---

## Pipeline Maintenance

### Versioning Your Pipeline

The Jenkinsfile evolves with the project. Treat it like code:

1. **Review pipeline changes in PRs.** Pipeline changes can break all builds. Review them carefully.
2. **Test pipeline changes on a feature branch.** Multibranch pipelines build every branch, so a Jenkinsfile change on `feature/update-pipeline` is tested without affecting `main`.
3. **Tag pipeline versions alongside application releases.** When you tag `v1.2.3`, the Jenkinsfile at that tag describes exactly how that version was built.

### Testing Pipelines

Use the **Pipeline Unit Testing Framework** to unit test Jenkinsfile logic:

```groovy
// test/PipelineTest.groovy
class PipelineTest extends BasePipelineTest {
    @Test
    void should_skip_deploy_on_feature_branch() {
        def script = loadScript('Jenkinsfile')
        binding.setVariable('BRANCH_NAME', 'feature/new-thing')
        script.run()

        // Verify deploy stage was skipped
        assertJobStatusSuccess()
        assertCallStackDoesNotContain('deploy.sh')
    }
}
```

### Documentation

Document your pipeline decisions:

```groovy
// Jenkinsfile

// WHY: Integration tests need Docker to spin up PostgreSQL.
// The 'docker-host' agent has the Docker daemon available.
// The default 'golang' agent runs inside Docker and cannot nest containers.
stage('Integration Tests') {
    agent { label 'docker-host' }
    steps { ... }
}

// WHY: We use a 5-minute timeout for production approval because
// unapproved deployments should not block the executor indefinitely.
// If nobody approves within 5 minutes, the build is marked as aborted
// and can be manually triggered again.
stage('Deploy Production') {
    options { timeout(time: 5, unit: 'MINUTES') }
    input { ... }
}
```

---

## Quick Reference: Best Practice Checklist

Use this checklist when setting up a new Jenkins pipeline:

```
PIPELINE SETUP
  [ ] Jenkinsfile committed to repository root
  [ ] Controller has 0 executors (builds on agents only)
  [ ] Build timeout set (pipeline + per-stage)
  [ ] Build history retention configured (keep last N)
  [ ] Concurrent builds disabled (or handled correctly)

SECURITY
  [ ] All secrets in Jenkins Credential Store
  [ ] No credentials in Jenkinsfile or Git history
  [ ] RBAC configured (admin, developer, viewer roles)
  [ ] CSRF protection enabled (default)
  [ ] Script approvals reviewed

QUALITY
  [ ] Lint stage runs before build
  [ ] Tests produce JUnit XML (published with junit step)
  [ ] Coverage threshold enforced
  [ ] Security scan (govulncheck, trivy) runs on every build

ARTIFACTS
  [ ] Binaries archived with fingerprinting
  [ ] Coverage report archived
  [ ] Test results published (junit step)
  [ ] Docker image tagged with commit SHA

NOTIFICATIONS
  [ ] Slack/email on failure
  [ ] GitHub commit status updated
  [ ] Build description set (coverage, image tag)

MAINTENANCE
  [ ] Docker cleanup in post/cleanup
  [ ] Workspace cleaned (cleanWs)
  [ ] Jenkins backed up (config + credentials + secrets)
  [ ] Go module cache persisted between builds
  [ ] Docker layer cache configured

OPERATIONS
  [ ] Quiet period set (avoid rapid-push build storms)
  [ ] Multibranch pipeline configured
  [ ] Branch protection requires CI status checks
  [ ] Monitoring configured (queue size, agent health)
```

---

## Summary

Jenkins is a powerful tool, but its flexibility means there are many ways to misconfigure it. The most impactful best practices are:

1. **Never run builds on the controller** — use agents
2. **Never store secrets in the Jenkinsfile** — use the credential store
3. **Always set timeouts** — stuck builds block everyone
4. **Always archive test results** — debugging failures should be fast
5. **Always clean up** — disk space is finite
6. **Always back up** — Jenkins configuration is irreplaceable
7. **Always use pipeline-as-code** — the Jenkinsfile lives in Git

These practices, combined with the pipeline design from Chapter 2 and the patterns from Chapter 8, create a robust, maintainable CI/CD system for the Claude Terminal MID Service.

---

**Previous:** [Chapter 8: Advanced Jenkins Patterns](08-advanced-jenkins-patterns.md)
**Next:** [Chapter 10: Jenkins Setup](10-jenkins-setup.md)
