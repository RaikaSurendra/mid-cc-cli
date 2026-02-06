# Chapter 1: What is Jenkins and Why

## What is CI/CD?

**CI/CD** stands for **Continuous Integration / Continuous Delivery** (or Deployment). It is a set of practices that automate how code moves from a developer's laptop to a running production system.

### The Car Assembly Line Analogy

Imagine a car factory before assembly lines existed. One worker would build an entire car from scratch — chassis, engine, wiring, paint, interior — all by hand. If something was wrong with the engine, you would not discover it until the car was fully assembled. Fixing it would mean tearing apart the entire vehicle.

Now imagine a modern assembly line:

```
[Chassis] --> [Engine Install] --> [Wiring] --> [Paint] --> [Interior] --> [Quality Check] --> [Ship]
    |              |                  |           |             |               |
    v              v                  v           v             v               v
  INSPECT        INSPECT           INSPECT     INSPECT       INSPECT      FINAL GATE
```

At every station, the work is inspected. If the engine does not fit, you find out immediately — not after the car is painted and upholstered. The line is automated, repeatable, and fast.

**CI/CD is the assembly line for software:**

| Assembly Line Step | CI/CD Equivalent |
|-------------------|------------------|
| Raw materials arrive | Developer pushes code to Git |
| Chassis welding | Code compiles (build stage) |
| Engine inspection | Automated tests run |
| Paint quality check | Linting and code quality gates |
| Final road test | Integration and smoke tests |
| Ship to dealer | Deploy to production |

**Continuous Integration (CI)** means every developer regularly merges their work into the main codebase, and an automated system builds and tests it immediately. Problems are caught within minutes, not days.

**Continuous Delivery (CD)** means the tested code is automatically packaged and ready to deploy at any time. With **Continuous Deployment**, it goes even further — code that passes all tests is automatically deployed to production without human intervention.

### Why CI/CD Matters for This Project

The Claude Terminal MID Service has two Go binaries, a Docker image, PostgreSQL integration, and ServiceNow components. Without CI/CD:

- A developer changes the session manager, forgets to run tests, and pushes code that breaks the ECC poller
- Someone builds the Docker image locally on macOS, but the production Alpine container behaves differently
- A security vulnerability sits in the code for weeks because nobody ran `golangci-lint`
- Deploying means SSH-ing into a server and running commands by hand, hoping nothing is missed

With CI/CD, every push triggers an automated pipeline that builds, tests, scans, and packages the service identically every time.

---

## What is Jenkins?

**Jenkins** is an open-source automation server written in Java. It orchestrates CI/CD pipelines — the automated workflows that build, test, and deploy your software.

### A Brief History

- **2004:** Kohsuke Kawaguchi at Sun Microsystems creates **Hudson**, an internal CI tool
- **2005:** Hudson is open-sourced and rapidly gains adoption
- **2010:** Oracle acquires Sun. The community forks Hudson into **Jenkins** due to trademark disputes
- **2011:** Jenkins becomes the dominant CI server, surpassing Hudson
- **2016:** Jenkins 2.0 launches with **Pipeline-as-Code** (Jenkinsfile), a transformative feature
- **2017:** Blue Ocean UI plugin modernizes the Jenkins interface
- **Today:** Jenkins has 1,800+ plugins, 300,000+ installations, and remains the most widely deployed CI/CD server in enterprise environments

### Why Jenkins is Still Dominant

Despite newer competitors (GitHub Actions, GitLab CI, CircleCI), Jenkins remains the most used CI/CD tool in enterprise environments for several reasons:

1. **On-premise control.** Jenkins runs on your infrastructure, not someone else's cloud. For organizations handling sensitive data or operating behind firewalls (like ServiceNow MID Server environments), this is non-negotiable.

2. **Plugin ecosystem.** With 1,800+ plugins, Jenkins integrates with virtually every tool, language, and platform. Need to talk to ServiceNow's API? There is a plugin. Need to build Docker images? Plugin. Need to notify a Slack channel? Plugin.

3. **Pipeline-as-Code.** The Jenkinsfile lives in your Git repository alongside your source code. Your CI/CD configuration is versioned, reviewed, and tracked like any other code.

4. **Flexibility.** Jenkins can build anything — Go binaries, Docker images, Node.js packages, mobile apps, firmware — all in the same installation.

5. **Community and documentation.** Decades of Stack Overflow answers, blog posts, and tutorials.

---

## Jenkins Architecture

### The Restaurant Analogy

Think of Jenkins as a busy restaurant kitchen:

```
                    +-------------------+
                    |   HEAD CHEF       |
                    |   (Controller)    |
                    |                   |
                    |  - Takes orders   |
                    |  - Assigns tasks  |
                    |  - Tracks status  |
                    +--------+----------+
                             |
              +--------------+--------------+
              |              |              |
     +--------v---+  +------v-----+  +-----v------+
     |  COOK #1   |  |  COOK #2   |  |  COOK #3   |
     |  (Agent)   |  |  (Agent)   |  |  (Agent)   |
     |            |  |            |  |            |
     | [Burner 1] |  | [Burner 1] |  | [Burner 1] |
     | [Burner 2] |  | [Burner 2] |  | [Burner 2] |
     | (Executors)|  | (Executors)|  | (Executors)|
     +------------+  +------------+  +------------+
```

| Restaurant Concept | Jenkins Concept | What It Does |
|-------------------|----------------|--------------|
| Head Chef | **Controller** (formerly "Master") | Manages the kitchen. Receives orders (build triggers), decides which cook handles what, tracks progress. Does NOT cook food itself. |
| Cook | **Agent** (formerly "Slave") | A worker machine that actually executes builds. Can be a VM, a Docker container, a bare-metal server, or a Kubernetes pod. |
| Stove Burner | **Executor** | A single slot on an agent that can run one build at a time. An agent with 2 executors can run 2 builds simultaneously. |
| Order Queue | **Build Queue** | When all cooks are busy, new orders wait here. First-in, first-out, with priority options. |
| Recipe | **Pipeline / Jenkinsfile** | The step-by-step instructions for building and testing the software. |
| Prep Station | **Workspace** | A directory on the agent where source code is checked out and the build happens. Each build gets a clean workspace. |
| Plated Dish | **Artifact** | The output of a build — a compiled binary, a Docker image, a test report. |

### Architecture Diagram

```
+------------------------------------------------------------------+
|                    JENKINS CONTROLLER                              |
|                                                                    |
|  +----------+  +----------+  +-----------+  +------------------+  |
|  |  Web UI  |  | REST API |  | Build     |  | Plugin Manager   |  |
|  |  (8080)  |  |          |  | Scheduler |  | (1800+ plugins)  |  |
|  +----------+  +----------+  +-----------+  +------------------+  |
|                                                                    |
|  +----------+  +----------+  +-----------+  +------------------+  |
|  |Credential|  | Pipeline |  | SCM       |  | Notification     |  |
|  |  Store   |  | Engine   |  | Poller    |  | System           |  |
|  +----------+  +----------+  +-----------+  +------------------+  |
+-----+---------------------+---------------------+----------------+
      |                      |                     |
      v                      v                     v
+------------+        +------------+        +------------+
| AGENT #1   |        | AGENT #2   |        | AGENT #3   |
| Linux VM   |        | Docker     |        | Kubernetes |
|            |        | Container  |        | Pod        |
| Go 1.24    |        | Go 1.24    |        | Go 1.24    |
| Docker CLI |        | Docker CLI |        | Docker CLI |
| 4 executors|        | 2 executors|        | 1 executor |
+------------+        +------------+        +------------+
```

The **Controller** never runs builds itself (this is a best practice — see Chapter 9). It:
- Hosts the web interface (port 8080)
- Stores configuration, credentials, and build history
- Schedules builds and assigns them to agents
- Manages the plugin ecosystem

**Agents** do the actual work:
- Check out source code from Git
- Run build commands (`make build`, `go test`, `docker build`)
- Report results back to the controller

---

## How a Build Works End-to-End

Here is exactly what happens when someone pushes code to the Claude Terminal MID Service repository:

```
Developer pushes code to GitHub
         |
         v
(1) TRIGGER: GitHub webhook hits Jenkins at /github-webhook/
         |
         v
(2) QUEUE: Jenkins creates a build request, places it in the Build Queue
         |
         v
(3) AGENT PICKUP: An idle agent with a free executor claims the build
         |
         v
(4) WORKSPACE: Agent creates a fresh directory: /workspace/mid-llm-cli/
         |
         v
(5) CHECKOUT: Agent runs `git clone` to pull the latest code
         |
         v
(6) STAGES: Agent executes the pipeline stages in order:
         |
         +---> [Deps]       go mod download
         +---> [Lint]        golangci-lint run
         +---> [Test]        go test -v ./...
         +---> [Build]       make build
         +---> [Docker]      docker build -t claude-terminal-service .
         +---> [Push]        docker push to registry
         |
         v
(7) ARTIFACTS: Agent archives binaries, test reports, coverage files
         |
         v
(8) NOTIFICATION: Jenkins posts build status to GitHub, sends Slack message
         |
         v
(9) CLEANUP: Workspace is cleaned, Docker images pruned
```

Each stage is defined in the `Jenkinsfile` at the root of the repository. If any stage fails, subsequent stages are skipped (fail-fast), and the developer is notified immediately.

---

## Jenkins vs. Alternatives

| Feature | Jenkins | GitHub Actions | GitLab CI |
|---------|---------|---------------|-----------|
| **Hosting** | Self-hosted (on-premise) | Cloud (GitHub-hosted) or self-hosted runners | Cloud (GitLab.com) or self-hosted |
| **Configuration** | Jenkinsfile (Groovy) | YAML workflow files | `.gitlab-ci.yml` (YAML) |
| **Plugin Ecosystem** | 1,800+ plugins | 20,000+ marketplace actions | Built-in features, fewer plugins |
| **Learning Curve** | Steeper — Java, Groovy, UI | Gentle — YAML, well-documented | Moderate — YAML, integrated |
| **Cost** | Free (open source) + infra costs | Free tier + pay per minute | Free tier + pay per minute |
| **On-premise** | Native | Requires self-hosted runners | Requires self-managed instance |
| **Container Support** | Via Docker plugin | Native (runs in containers) | Native (runs in containers) |
| **Pipeline-as-Code** | Jenkinsfile in repo | `.github/workflows/*.yml` | `.gitlab-ci.yml` |
| **Scalability** | Agent-based, manual scaling | Auto-scaling runners | Auto-scaling runners |
| **UI** | Functional (Blue Ocean improves it) | Modern, integrated with GitHub | Modern, integrated with GitLab |
| **Enterprise Features** | RBAC via plugins, LDAP | Enterprise plan required | Enterprise plan required |

### When Each Shines

- **GitHub Actions:** Best when your code already lives on GitHub and you want zero infrastructure overhead
- **GitLab CI:** Best when you want a single platform for code, CI, CD, registry, and monitoring
- **Jenkins:** Best when you need on-premise control, complex multi-tool workflows, or integration with legacy systems

---

## Why Jenkins for This Project

The Claude Terminal MID Service has specific requirements that make Jenkins a strong fit:

1. **On-premise alignment.** This project runs alongside ServiceNow MID Servers, which are inherently on-premise. Jenkins deploys on the same infrastructure, keeping builds inside the network perimeter. No code or credentials leave the network.

2. **MID Server integration.** Jenkins can trigger ServiceNow REST APIs, validate MID Server connectivity, and run integration tests against a local ServiceNow instance — all through its plugin ecosystem.

3. **Docker build pipeline.** The project's multi-stage Dockerfile produces a production image with Go binaries and Claude CLI. Jenkins's Docker Pipeline plugin handles building, tagging, and pushing with native support.

4. **Multi-binary builds.** The project produces two binaries (`claude-terminal-service` and `ecc-poller`). Jenkins easily handles parallel builds and separate artifact archival.

5. **Database integration testing.** The pipeline needs to spin up PostgreSQL for integration tests. Jenkins can orchestrate Docker Compose services as part of the build.

6. **Credential management.** The project requires ServiceNow API credentials, encryption keys, database passwords, and Docker registry tokens. Jenkins's credential store handles these securely with audit trails.

7. **Organizational adoption.** Many ServiceNow enterprises already run Jenkins. Adding this project to an existing Jenkins installation is simpler than introducing a new CI/CD platform.

---

## Key Terminology Glossary

| Term | Definition | Example in This Project |
|------|-----------|------------------------|
| **Job** | A single unit of work configured in Jenkins. Can be a freestyle project, a pipeline, or a multibranch pipeline. | "mid-llm-cli-pipeline" |
| **Pipeline** | A series of automated stages that code passes through from commit to deployment. Defined in a Jenkinsfile. | Checkout -> Lint -> Test -> Build -> Docker -> Deploy |
| **Stage** | A logical grouping of steps within a pipeline. Shown as a column in the pipeline visualization. | "Unit Tests", "Docker Build", "Deploy Staging" |
| **Step** | A single task within a stage. The smallest unit of work. | `sh 'make build'` or `sh 'go test ./...'` |
| **Node** | A machine (controller or agent) that Jenkins can run builds on. | A Linux VM with Go 1.24 and Docker installed |
| **Agent** | A worker machine that executes builds on behalf of the controller. | A Docker container running `golang:1.24-alpine` |
| **Executor** | A slot on a node that can run one build at a time. A node with N executors can run N concurrent builds. | Agent with 4 executors = 4 simultaneous builds |
| **Workspace** | A directory on the agent where the build runs. Contains the checked-out source code and build outputs. | `/workspace/mid-llm-cli/` |
| **Artifact** | A file produced by a build that is archived for later use. | `bin/claude-terminal-service`, `coverage.html` |
| **SCM** | Source Code Management — the version control system. Jenkins supports Git, SVN, Mercurial, and others. | Git (GitHub repository) |
| **Webhook** | An HTTP callback that notifies Jenkins when code is pushed. Eliminates the need for Jenkins to poll the repository. | GitHub sends POST to `https://jenkins.example.com/github-webhook/` |
| **Trigger** | An event that starts a pipeline. Can be a webhook, a schedule (cron), a manual click, or another pipeline. | Push to `main` branch triggers the build |
| **Credentials** | Secrets stored in Jenkins (passwords, API tokens, SSH keys, certificates). Never hardcoded in the Jenkinsfile. | `SERVICENOW_API_PASSWORD`, `DOCKER_REGISTRY_TOKEN` |
| **Jenkinsfile** | A text file in the repository root that defines the pipeline. Written in Groovy (Declarative or Scripted syntax). | `/Jenkinsfile` |
| **Declarative Pipeline** | A structured, opinionated syntax for Jenkinsfiles. Easier to read and write. | `pipeline { agent any; stages { ... } }` |
| **Scripted Pipeline** | A flexible, Groovy-based syntax for Jenkinsfiles. More powerful but harder to maintain. | `node { stage('Build') { sh 'make build' } }` |
| **Multibranch Pipeline** | A Jenkins job type that automatically discovers branches and PRs in a repository, creating a pipeline for each. | Branches `main`, `develop`, and `feature/websocket` each get their own pipeline |
| **Blue Ocean** | A modern Jenkins UI plugin that provides visual pipeline editing and improved build visualization. | Dashboard showing pipeline stages as colored boxes |
| **Shared Library** | A reusable Groovy library stored in a separate repository. Common pipeline logic is written once and shared across projects. | A library containing `buildGoProject()` function |
| **Post Section** | Actions that run after stages complete, regardless of success or failure. Used for cleanup, notifications, and artifact archival. | Send Slack notification on failure, always clean up Docker images |
| **Parameters** | User-configurable inputs for a pipeline. Displayed as a form before the build starts. | `DEPLOY_ENV` (choice: staging/production), `SKIP_TESTS` (boolean) |
| **Stash/Unstash** | Mechanism to pass files between stages that may run on different agents. Stash saves files; unstash retrieves them. | Stash built binaries in "Build" stage, unstash in "Docker Build" stage |

---

## Summary

Jenkins is an automation server that acts as the assembly line for your software. It receives code changes, runs them through a series of automated stages (build, test, scan, deploy), and reports the results. For the Claude Terminal MID Service, Jenkins provides on-premise control, Docker integration, credential management, and the flexibility to orchestrate a complex multi-binary, multi-service build pipeline.

In the next chapter, we will design the specific CI/CD pipeline for this project — mapping each stage to the project's build, test, and deployment requirements.

---

**Next:** [Chapter 2: CI/CD Pipeline Design for This Project](02-cicd-pipeline-design.md)
