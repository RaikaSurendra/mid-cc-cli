# Chapter 13: Webhooks & Triggers

This chapter covers every way to trigger the Claude Terminal MID Service pipeline -- from clicking a button to automatic GitHub webhooks. You will learn how to set up each trigger type and when to use which one.

---

## Trigger Types Overview

| Trigger | When It Fires | Latency | Setup Effort |
|---------|--------------|---------|-------------|
| Manual ("Build Now") | When you click the button | Instant | None |
| SCM Polling | Jenkins checks Git on a schedule | Up to poll interval | Low |
| GitHub Webhook | GitHub notifies Jenkins on push | ~1 second | Medium |
| Cron Schedule | On a time schedule | At scheduled time | Low |
| Upstream Job | After another job completes | Instant | Low |
| Parameterized | Manual with input parameters | Instant | Low |

---

## 1. Manual Trigger: "Build Now"

The simplest trigger. You log into Jenkins and click a button.

### How to Use

1. Open the pipeline job (e.g., `claude-terminal-service`)
2. Click **Build Now** in the left sidebar (for a basic build)
3. Or click **Build with Parameters** to set `DEPLOY_ENV` and `SKIP_TESTS`

### When to Use

- First time running the pipeline after setup
- Testing Jenkinsfile changes
- Deploying to staging/production (with `DEPLOY_ENV` parameter)
- Debugging a failed build by re-running it

### What Happens

Jenkins checks out the latest code from the configured branch, runs through all stages in the `Jenkinsfile`, and reports results. The build appears in the build history with a number (e.g., #1, #2, #3).

---

## 2. SCM Polling

Jenkins periodically checks the Git repository for new commits. If it finds changes since the last build, it triggers a new build.

### How to Configure

Add this to the `Jenkinsfile`:

```groovy
pipeline {
    triggers {
        pollSCM('H/5 * * * *')
    }
    // ... rest of pipeline
}
```

The `H/5 * * * *` cron expression means "every 5 minutes, with a hash-based offset." The `H` distributes load so that multiple projects polling the same Git server do not all poll at exactly the same moment.

Alternatively, configure it in the Jenkins UI:

1. Open the job configuration
2. Under **Build Triggers**, check **Poll SCM**
3. Enter the schedule: `H/5 * * * *`
4. Click **Save**

### Pros and Cons

| Pros | Cons |
|------|------|
| Works with any Git host (GitHub, GitLab, Bitbucket, self-hosted) | Delay of up to 5 minutes between push and build |
| No firewall or network configuration needed | Wastes resources polling when nothing changed |
| Simple to set up | Adds load to the Git server with frequent polls |

### When to Use

- When your Git host cannot send webhooks (e.g., behind a firewall)
- As a fallback if webhooks are unreliable
- For repositories with infrequent commits (set a longer interval like `H/15 * * * *`)

---

## 3. GitHub Webhook (Recommended)

GitHub sends an HTTP POST to Jenkins whenever code is pushed. This is the fastest and most efficient trigger.

### Step 1: Configure Jenkins

1. Open the pipeline job configuration
2. Under **Build Triggers**, check **GitHub hook trigger for GITScm polling**
3. Click **Save**

This tells Jenkins to listen for incoming GitHub webhook events.

### Step 2: Get Your Jenkins URL

Your Jenkins instance must be reachable from the internet (GitHub needs to send it HTTP requests). The URL looks like:

```
https://jenkins.yourcompany.com
```

If running locally with Docker for learning, you need a tunnel:

```bash
# Using ngrok (for local testing only)
ngrok http 8080
# This gives you a URL like https://abc123.ngrok.io
```

### Step 3: Create the Webhook in GitHub

1. Open your repository on GitHub: `https://github.com/yourorg/mid-llm-cli`
2. Click **Settings** (tab)
3. Click **Webhooks** (left sidebar)
4. Click **Add webhook**
5. Fill in:

| Field | Value |
|-------|-------|
| Payload URL | `https://jenkins.yourcompany.com/github-webhook/` |
| Content type | `application/json` |
| Secret | (optional but recommended -- a shared secret for HMAC verification) |
| Which events? | Select **Just the push event** (or **Let me select individual events** and check Push and Pull Request) |
| Active | Checked |

6. Click **Add webhook**

### Step 4: Verify

Push a commit to the repository:

```bash
git commit --allow-empty -m "Test webhook trigger"
git push origin main
```

Within 1-2 seconds, a new build should appear in Jenkins. Check the webhook delivery log in GitHub (**Settings > Webhooks > Recent Deliveries**) to confirm the POST was sent and received a 200 response.

### Troubleshooting Webhooks

| Problem | Solution |
|---------|----------|
| GitHub shows "Connection refused" | Jenkins is not reachable from the internet. Check firewall rules or use ngrok for testing. |
| GitHub shows 403 Forbidden | Jenkins CSRF protection is blocking the request. Navigate to **Manage Jenkins > Security** and ensure "Enable proxy compatibility" is checked. |
| GitHub shows 200 but no build starts | The job is not configured with "GitHub hook trigger for GITScm polling." Check the build trigger settings. |
| Webhook fires but wrong branch builds | Check the branch specifier in the job or Multibranch Pipeline configuration. |

---

## 4. Multibranch Pipeline (Recommended for Teams)

A Multibranch Pipeline automatically discovers branches in your Git repository and creates a separate pipeline for each branch that contains a `Jenkinsfile`. This is the most powerful trigger setup for team development.

### How to Set Up for This Project

#### Step 1: Create the Job

1. From the Jenkins dashboard, click **New Item**
2. Enter the name: `claude-terminal-service`
3. Select **Multibranch Pipeline**
4. Click **OK**

#### Step 2: Configure the Branch Source

1. Under **Branch Sources**, click **Add source** > **GitHub**
2. Configure:

| Field | Value |
|-------|-------|
| Credentials | (Your GitHub personal access token or SSH key) |
| Repository HTTPS URL | `https://github.com/yourorg/mid-llm-cli.git` |

3. Under **Behaviours**, click **Add** and select:
   - **Discover branches**: Strategy = "All branches"
   - **Discover pull requests from origin**: Strategy = "Merging the pull request with the current target branch revision"

#### Step 3: Set Branch Filtering

Under **Branch Sources > Add** > **Filter by name (with regular expression)**:

```
main|develop|feature/.*
```

This tells Jenkins to only create pipelines for:
- `main` -- Production branch (full pipeline including Docker push)
- `develop` -- Integration branch (full pipeline, no Docker push)
- `feature/*` -- Feature branches (tests only, no Docker push or deploy)

Branches not matching this pattern are ignored.

#### Step 4: Configure Build Strategies

Under **Build Configuration**:

| Field | Value |
|-------|-------|
| Script Path | `Jenkinsfile` |
| Lightweight checkout | Checked |

Under **Orphaned Item Strategy**:

| Field | Value |
|-------|-------|
| Days to keep old items | 7 |
| Max # of old items to keep | 5 |

This automatically deletes pipeline jobs for branches that have been deleted from Git.

#### Step 5: Save and Scan

Click **Save**. Jenkins immediately scans the repository and creates a pipeline for each matching branch. You will see:

```
claude-terminal-service/
  main        #1 (building)
  develop     #1 (building)
  feature/add-websocket  #1 (building)
```

### How Feature Branch Builds Work

When a developer creates a feature branch and pushes it:

```bash
git checkout -b feature/session-timeout-fix
# ... make changes ...
git push origin feature/session-timeout-fix
```

Jenkins automatically:
1. Detects the new branch (via webhook or SCM polling)
2. Creates a new pipeline job for `feature/session-timeout-fix`
3. Runs the `Jenkinsfile` on that branch
4. Reports the result

Because the `Jenkinsfile` has `when { branch 'main' }` on Docker Push and Deploy stages, feature branches only run: Checkout, Dependencies, Quality Gates, Unit Tests, Race Detection, Coverage, and Build. They skip Docker Push and Deploy.

### Pull Request Builds

When a developer opens a pull request from `feature/session-timeout-fix` to `main`:

1. Jenkins detects the PR (via the "Discover pull requests" behaviour)
2. It creates a temporary pipeline that merges the PR branch into `main` and runs the full test suite
3. The build result is posted back to GitHub as a status check

This means the PR page on GitHub shows:

```
  claude-terminal-service/PR-42: Checks passed (green checkmark)
  All 142 tests passed, 72.3% coverage
```

Or if something fails:

```
  claude-terminal-service/PR-42: Checks failed (red X)
  2 tests failed in internal/session
```

### Posting Status to GitHub

For Jenkins to post build status back to GitHub:

1. Install the **GitHub** plugin (already in our plugins list)
2. Create a **GitHub Personal Access Token** with `repo:status` scope
3. Add it as a Jenkins credential (Secret text, ID: `github-token`)
4. Configure in **Manage Jenkins > System > GitHub Servers**:
   - API URL: `https://api.github.com`
   - Credentials: Select the `github-token` credential
   - Click **Test connection** to verify

---

## 5. Cron Triggers

Run the pipeline on a fixed time schedule, regardless of code changes.

### Nightly Builds

The `Jenkinsfile.nightly` uses this trigger:

```groovy
triggers {
    cron('H 2 * * *')
}
```

This means "run once a day at approximately 2 AM." The `H` hash distributes the exact minute randomly (e.g., 2:17 AM) to avoid thundering herd problems on the Jenkins server.

### Cron Syntax

```
MINUTE HOUR DOM MONTH DOW
  |      |   |    |     |
  |      |   |    |     +-- Day of week (0-7, where 0 and 7 = Sunday)
  |      |   |    +-------- Month (1-12)
  |      |   +------------- Day of month (1-31)
  |      +------------------ Hour (0-23)
  +------------------------- Minute (0-59)
```

Examples for the Claude Terminal MID Service:

| Schedule | Cron Expression | Use Case |
|----------|----------------|----------|
| Every weekday at 2 AM | `H 2 * * 1-5` | Nightly regression tests |
| Every Monday at 6 AM | `H 6 * * 1` | Weekly security scan |
| Every 4 hours | `H H/4 * * *` | Frequent integration checks |
| First day of month | `H 2 1 * *` | Monthly performance baseline |

### How to Set Up the Nightly Pipeline

1. Create a new **Pipeline** job named `claude-terminal-service-nightly`
2. Under **Pipeline**, select **Pipeline script from SCM**
3. Set the SCM to Git with your repository URL
4. Set the script path to `Jenkinsfile.nightly`
5. Click **Save**

The `triggers { cron('H 2 * * *') }` directive in `Jenkinsfile.nightly` automatically registers the cron schedule. Jenkins will run the nightly pipeline every day at approximately 2 AM.

---

## 6. Upstream Triggers

Run one job after another completes. Useful when you have separate pipelines for the terminal service and the ECC poller.

### Example: Build ECC Poller After Terminal Service Passes

If you split the project into two separate pipeline jobs:

```groovy
// In the ecc-poller Jenkinsfile:
pipeline {
    triggers {
        upstream(
            upstreamProjects: 'claude-terminal-service/main',
            threshold: hudson.model.Result.SUCCESS
        )
    }
    // ...
}
```

This triggers the ECC Poller pipeline whenever the `claude-terminal-service/main` pipeline succeeds. The `threshold` parameter controls the minimum result:

| Threshold | Triggers When |
|-----------|--------------|
| `SUCCESS` | Only on success (green) |
| `UNSTABLE` | On success or unstable (green or yellow) |
| `FAILURE` | On success, unstable, or failure (always) |

For this project, since both binaries come from the same repository, you typically use a single Jenkinsfile that builds both. Upstream triggers are more useful if you split the project into separate repositories.

---

## 7. Parameterized Builds

The project's `Jenkinsfile` already includes parameters:

```groovy
parameters {
    choice(name: 'DEPLOY_ENV', choices: ['none', 'staging', 'production'], ...)
    booleanParam(name: 'SKIP_TESTS', defaultValue: false, ...)
}
```

### Deploy to a Specific Environment

To deploy to staging:

1. Open the pipeline job
2. Click **Build with Parameters**
3. Set `DEPLOY_ENV` to `staging`
4. Click **Build**

The Deploy Staging stage will run because the `when` block checks:

```groovy
when {
    allOf {
        branch 'main'
        expression { return params.DEPLOY_ENV == 'staging' || params.DEPLOY_ENV == 'production' }
    }
}
```

### Emergency Hotfix Without Tests

To push a critical fix without waiting for tests:

1. Click **Build with Parameters**
2. Check `SKIP_TESTS`
3. Set `DEPLOY_ENV` to `staging`
4. Click **Build**

All test stages (Quality Gates, Unit Tests, Race Detection, Coverage, Integration Tests) are skipped. The pipeline goes directly from Dependencies to Build to Docker Build to Deploy.

**Use this sparingly.** Every skipped test is a risk. When the emergency is resolved, run a full build to verify nothing is broken.

### Adding More Parameters

You can extend the `Jenkinsfile` with additional parameters:

```groovy
parameters {
    choice(name: 'DEPLOY_ENV', choices: ['none', 'staging', 'production'], ...)
    booleanParam(name: 'SKIP_TESTS', defaultValue: false, ...)
    // Additional parameters for this project:
    string(
        name: 'IMAGE_TAG_OVERRIDE',
        defaultValue: '',
        description: 'Override the Docker image tag (default: git SHA)'
    )
    booleanParam(
        name: 'FORCE_PUSH',
        defaultValue: false,
        description: 'Push Docker image even on non-main branches'
    )
}
```

---

## Recommended Trigger Setup for This Project

Here is the trigger configuration that covers all use cases:

### Main Pipeline (Jenkinsfile)

| Trigger | Configuration | Purpose |
|---------|--------------|---------|
| GitHub Webhook | `GitHub hook trigger for GITScm polling` | Instant builds on every push |
| SCM Polling (backup) | `pollSCM('H/5 * * * *')` | Catches missed webhooks |
| Manual | "Build with Parameters" | Deployments and hotfixes |

### Nightly Pipeline (Jenkinsfile.nightly)

| Trigger | Configuration | Purpose |
|---------|--------------|---------|
| Cron | `cron('H 2 * * *')` | Full regression + security scan every night |

### Multibranch Pipeline

| Branch Pattern | Builds | Docker Push | Deploy |
|----------------|--------|-------------|--------|
| `main` | All stages | Yes | Yes (with parameter) |
| `develop` | All stages | No | No |
| `feature/*` | Tests + Build only | No | No |
| PR to main | Tests + Build only | No | No |

### Complete Trigger Block

If you want to add both webhook and polling as a backup, add this to the `Jenkinsfile`:

```groovy
pipeline {
    triggers {
        // Primary: GitHub webhook (requires webhook setup in GitHub)
        // Backup: Poll every 5 minutes in case webhooks fail
        pollSCM('H/5 * * * *')
    }
    // ... rest of pipeline
}
```

The GitHub webhook trigger is configured in the job settings (not in the Jenkinsfile), so you enable it via the UI checkbox. The `pollSCM` in the Jenkinsfile serves as a backup.

---

## Summary

| Use Case | Best Trigger | Configuration |
|----------|-------------|---------------|
| Every push to any branch | GitHub Webhook + Multibranch Pipeline | Webhook + Multibranch job |
| Nightly regression tests | Cron | `cron('H 2 * * *')` in `Jenkinsfile.nightly` |
| Deploy to staging | Manual with parameters | "Build with Parameters" > DEPLOY_ENV=staging |
| Emergency hotfix | Manual with SKIP_TESTS | "Build with Parameters" > SKIP_TESTS=true |
| PR validation | Multibranch Pipeline | "Discover pull requests" behaviour |
| After another job passes | Upstream trigger | `upstream(upstreamProjects: '...')` |

---

Next: Continue to the next chapter in the Jenkins guide for advanced topics like pipeline shared libraries and production hardening.
