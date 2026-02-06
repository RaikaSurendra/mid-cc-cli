# Chapter 6: Docker Build & Push

## How the Dockerfile Works

This project uses a **multi-stage Docker build**, which is a technique for creating small, secure production images. The idea is simple: use one container to compile the code (the "builder" stage), then copy only the compiled binaries into a minimal runtime container. The source code, compiler, and build tools are discarded.

The Dockerfile has two stages:

### Stage 1: Builder

```dockerfile
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN make build
```

**Line-by-line explanation:**

1. **`FROM golang:1.24-alpine AS builder`** -- Starts from the official Go 1.24 image based on Alpine Linux. The `AS builder` gives this stage a name so the second stage can reference it. This image is ~300 MB and contains the Go compiler, standard library, and Alpine package manager.

2. **`RUN apk add --no-cache git make`** -- Installs Git (needed by `go mod download` for some dependencies that use Git protocols) and Make (needed to run `make build`). The `--no-cache` flag skips caching the package index, keeping the layer small.

3. **`WORKDIR /app`** -- Sets the working directory inside the container. All subsequent commands run from `/app`.

4. **`COPY go.mod go.sum ./`** -- Copies only the dependency files first. This is a **Docker layer caching optimization**. Since dependencies change rarely, this layer is cached and reused across builds. Only when `go.mod` or `go.sum` change does Docker need to re-run the next step.

5. **`RUN go mod download`** -- Downloads all dependencies into the module cache inside the container. Because of the layer caching above, this step is skipped if dependencies have not changed since the last build.

6. **`COPY . .`** -- Copies the entire project source code into the container. This layer changes on every code commit, but the dependency layer above is already cached.

7. **`RUN make build`** -- Compiles both `claude-terminal-service` and `ecc-poller` into `bin/`. Uses the `-ldflags "-s -w"` flags for stripped production binaries.

### Stage 2: Runtime

```dockerfile
FROM alpine:latest

RUN apk add --no-cache \
    ca-certificates \
    bash \
    curl \
    nodejs \
    npm \
    git

RUN npm install -g @anthropic-ai/claude-code

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

RUN mkdir -p /app /tmp/claude-sessions /var/log && \
    chown -R appuser:appgroup /app /tmp/claude-sessions /var/log

COPY --from=builder /app/bin/claude-terminal-service /app/
COPY --from=builder /app/bin/ecc-poller /app/

COPY --chown=appuser:appgroup .env.example /app/.env.example

USER appuser
WORKDIR /app

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:3000/health || exit 1

CMD ["./claude-terminal-service"]
```

**Line-by-line explanation:**

1. **`FROM alpine:latest`** -- Starts a fresh, minimal image (~5 MB). The Go compiler and all build artifacts from Stage 1 are completely gone. Only what we explicitly `COPY` into this stage will exist.

2. **`RUN apk add --no-cache ca-certificates bash curl nodejs npm git`** -- Installs runtime dependencies:
   - `ca-certificates`: TLS certificate bundle for HTTPS connections to ServiceNow
   - `bash`: Required by Claude CLI and shell scripts
   - `curl`: Used by the HEALTHCHECK directive
   - `nodejs` + `npm`: Required by Claude Code CLI (it is a Node.js application)
   - `git`: Required by Claude Code CLI for repository operations

3. **`RUN npm install -g @anthropic-ai/claude-code`** -- Installs the Claude Code CLI globally. This is the actual AI tool that the service wraps with PTY sessions.

4. **`RUN addgroup -S appgroup && adduser -S appuser -G appgroup`** -- Creates a non-root user and group. The `-S` flag creates a "system" user with no home directory or login shell. Running as non-root is a security best practice.

5. **`RUN mkdir -p ... && chown -R ...`** -- Creates the directories the application needs and gives ownership to the non-root user:
   - `/app`: Where the binaries live
   - `/tmp/claude-sessions`: Where PTY session workspaces are created
   - `/var/log`: Where log files are written

6. **`COPY --from=builder /app/bin/claude-terminal-service /app/`** -- Copies the compiled HTTP service binary from the builder stage. The `--from=builder` reference is how multi-stage builds work -- you reach back into a previous stage's filesystem.

7. **`COPY --from=builder /app/bin/ecc-poller /app/`** -- Copies the compiled ECC poller binary.

8. **`COPY --chown=appuser:appgroup .env.example /app/.env.example`** -- Copies the example configuration file, owned by the app user. In production, the real `.env` is mounted via Docker volumes or environment variables.

9. **`USER appuser`** -- Switches to the non-root user. All subsequent commands (and the container at runtime) run as this user.

10. **`EXPOSE 3000`** -- Documents that the container listens on port 3000. This does not actually publish the port -- it is metadata used by Docker Compose and orchestrators.

11. **`HEALTHCHECK`** -- Configures Docker's built-in health checking:
    - Checks every 30 seconds
    - Considers a check failed after 10 seconds
    - Waits 5 seconds before the first check (startup grace period)
    - Marks the container as unhealthy after 3 consecutive failures
    - Uses `curl` to hit the `/health` endpoint

12. **`CMD ["./claude-terminal-service"]`** -- Default command when the container starts. For the ECC poller container, this is overridden in `docker-compose.yml` with `command: ["./ecc-poller"]`.

---

## What Gets Installed in the Runtime Image

| Component | Size (approx.) | Purpose |
|-----------|----------------|---------|
| Alpine Linux base | ~5 MB | Minimal OS |
| ca-certificates | ~1 MB | TLS for HTTPS to ServiceNow |
| bash | ~2 MB | Shell for Claude CLI |
| curl | ~3 MB | Health checks |
| Node.js + npm | ~50 MB | Runtime for Claude Code CLI |
| git | ~12 MB | Used by Claude CLI for repo ops |
| Claude Code CLI | ~30 MB | The AI tool this service wraps |
| claude-terminal-service | ~18 MB | HTTP API binary |
| ecc-poller | ~15 MB | ECC Queue poller binary |
| **Total** | **~136 MB** | |

The builder stage (with Go compiler, source code, and build cache) would be ~800 MB. The multi-stage build reduces the final image to roughly 1/6th of that size.

---

## Image Tagging Strategy

Docker images need tags so you can identify which version is running. A good tagging strategy uses multiple tags per build:

### Recommended Tags for Jenkins

```bash
# 1. Git commit SHA -- the only truly unique identifier
docker tag claude-terminal-service:latest claude-terminal-service:abc1234

# 2. Semantic version (when releasing)
docker tag claude-terminal-service:latest claude-terminal-service:1.2.3

# 3. Latest -- always points to the most recent build on main
docker tag claude-terminal-service:latest claude-terminal-service:latest

# 4. Branch name -- useful for feature branch testing
docker tag claude-terminal-service:latest claude-terminal-service:feature-xyz
```

| Tag Type | Example | When to Use | Mutable? |
|----------|---------|------------|----------|
| Git SHA | `abc1234def` | Every build | No -- one SHA = one image forever |
| Semver | `1.2.3` | Release builds | No -- once published, a version is final |
| Latest | `latest` | Main branch builds | Yes -- overwritten on each build |
| Branch | `feature/auth-fix` | Feature branch builds | Yes -- overwritten on each push to branch |

**In Jenkins, generate the Git SHA tag dynamically:**

```groovy
environment {
    GIT_SHA = sh(script: 'git rev-parse --short HEAD', returnStdout: true).trim()
    IMAGE_NAME = 'claude-terminal-service'
}
```

---

## Docker Layer Caching for Faster CI Builds

Docker builds each Dockerfile instruction as a "layer." If a layer has not changed since the last build, Docker reuses the cached version. This is why the Dockerfile copies `go.mod` and `go.sum` before copying source code:

```dockerfile
# Layer 1: Rarely changes -- cached most of the time
COPY go.mod go.sum ./
RUN go mod download

# Layer 2: Changes on every commit -- never cached
COPY . .
RUN make build
```

### Enabling Cache in Jenkins

By default, Jenkins Docker builds on ephemeral agents have no cache. Each build starts fresh. To enable caching:

**Option 1: Docker BuildKit inline cache**

```groovy
stage('Docker Build') {
    steps {
        sh '''
            DOCKER_BUILDKIT=1 docker build \
                --cache-from ${IMAGE_NAME}:latest \
                --build-arg BUILDKIT_INLINE_CACHE=1 \
                -t ${IMAGE_NAME}:${GIT_SHA} \
                -t ${IMAGE_NAME}:latest \
                .
        '''
    }
}
```

This tells Docker to use the previously pushed `latest` image as a cache source. Layers that match are reused.

**Option 2: Persistent Docker volume for Go module cache**

```groovy
agent {
    docker {
        image 'golang:1.24-alpine'
        args '-v go-mod-cache:/go/pkg/mod -v docker-layer-cache:/var/lib/docker'
    }
}
```

**Impact**: A clean build takes ~3-5 minutes (downloading dependencies, installing Node.js). A cached build takes ~30-60 seconds (only recompiling changed Go code).

---

## How docker-compose.yml Orchestrates Services

The `docker-compose.yml` defines 4 services that together form the complete system:

```
                    +-------------------+
                    |   PostgreSQL      |
                    |   (port 5432)     |
                    +---------+---------+
                              |
                    +---------v---------+
                    | claude-terminal-  |
                    | service (port 3000)|
                    +---------+---------+
                              |
                    +---------v---------+
                    |   ecc-poller      |
                    |   (no port)       |
                    +---------+---------+
                              |
                    +---------v---------+
                    | ServiceNow MID    |
                    | Server            |
                    +-------------------+
```

### Service 1: PostgreSQL

```yaml
postgres:
    image: postgres:15-alpine
    environment:
        POSTGRES_USER: claude_user
        POSTGRES_PASSWORD: claude_password
        POSTGRES_DB: claude_terminal
    ports:
        - "5433:5432"
    healthcheck:
        test: ["CMD-SHELL", "pg_isready -U claude_user -d claude_terminal"]
```

- Uses the official PostgreSQL 15 image
- Maps to host port 5433 (avoids conflict if you have a local PostgreSQL on 5432)
- Health check uses `pg_isready` to verify the database is accepting connections
- Stores data in a Docker volume (`postgres-data`) so it survives container restarts

### Service 2: Claude Terminal Service

```yaml
claude-terminal-service:
    build:
        context: .
        dockerfile: Dockerfile
    env_file: .env
    environment:
        DB_HOST: "postgres"
        DB_PORT: "5432"
    depends_on:
        postgres:
            condition: service_healthy
```

- Builds from the project Dockerfile
- Loads configuration from `.env` file, overriding DB settings to point to the Docker Compose PostgreSQL
- **`depends_on` with `condition: service_healthy`** means Docker Compose waits until PostgreSQL's health check passes before starting this service. This prevents startup crashes from trying to connect to a database that is still initializing.

### Service 3: ECC Poller

```yaml
ecc-poller:
    build:
        context: .
        dockerfile: Dockerfile
    command: ["./ecc-poller"]
    environment:
        NODE_SERVICE_HOST: "claude-terminal-service"
    depends_on:
        claude-terminal-service:
            condition: service_healthy
```

- Uses the **same Dockerfile** as the terminal service (both binaries are in the image)
- Overrides the default command from `./claude-terminal-service` to `./ecc-poller`
- Sets `NODE_SERVICE_HOST` to the Docker Compose service name, so the poller can reach the HTTP service via Docker's internal DNS
- Waits for the terminal service to be healthy before starting

### Service 4: ServiceNow MID Server

```yaml
servicenow-mid-server:
    image: servicenow-mid-server:zurich-patch4-hotfix3
    deploy:
        resources:
            limits:
                cpus: '2.0'
                memory: 4G
            reservations:
                cpus: '0.5'
                memory: 1G
```

- Uses a pre-built MID Server image
- Resource limits prevent the Java-based MID Server from consuming all host resources
- Separate health check with a 3-minute start period (MID Server is slow to initialize)

### Network

All four services are on the same Docker bridge network (`mid-network`), allowing them to communicate via service names as DNS hostnames.

### Running for Integration Tests in Jenkins

```groovy
stage('Integration Tests') {
    steps {
        sh 'docker compose up -d --build'
        sh 'docker compose ps'

        // Wait for services to be healthy
        sh '''
            for i in $(seq 1 30); do
                if curl -sf http://localhost:3000/health; then
                    echo "Service is healthy"
                    break
                fi
                echo "Waiting for service... ($i/30)"
                sleep 5
            done
        '''

        // Run integration tests against the live service
        sh 'curl -sf http://localhost:3000/health | jq .'

        // Cleanup
        sh 'docker compose down -v'
    }
}
```

---

## Docker Registry Concepts

A Docker registry is a server that stores and distributes Docker images. When Jenkins builds an image, it pushes the image to a registry. When Kubernetes (or another host) deploys the image, it pulls from the registry.

### Common Registries

| Registry | URL | Use Case |
|----------|-----|----------|
| **Docker Hub** | `docker.io` | Public images, open-source projects |
| **AWS ECR** | `<account>.dkr.ecr.<region>.amazonaws.com` | AWS deployments |
| **GCP GCR/Artifact Registry** | `gcr.io/<project>` or `<region>-docker.pkg.dev` | Google Cloud deployments |
| **Azure ACR** | `<registry>.azurecr.io` | Azure deployments |
| **Self-hosted** | Your own server running Harbor, Nexus, or GitLab Registry | Air-gapped / on-prem environments |

### Push and Pull Commands

```bash
# Tag the image for the target registry
docker tag claude-terminal-service:latest registry.example.com/midserver/claude-terminal-service:1.2.3

# Authenticate to the registry
docker login registry.example.com

# Push the image
docker push registry.example.com/midserver/claude-terminal-service:1.2.3

# Pull the image (on the deployment host)
docker pull registry.example.com/midserver/claude-terminal-service:1.2.3
```

---

## How the Jenkins Docker Pipeline Plugin Works

The Jenkins Docker Pipeline plugin provides Groovy methods for building, tagging, and pushing images directly in your Jenkinsfile:

```groovy
stage('Docker Build & Push') {
    steps {
        script {
            // Build the image
            def image = docker.build("claude-terminal-service:${GIT_SHA}")

            // Push to registry with credentials
            docker.withRegistry('https://registry.example.com', 'docker-registry-credentials') {
                image.push("${GIT_SHA}")   // Push with SHA tag
                image.push("latest")        // Push with latest tag

                // Push semver tag only on release branches
                if (env.BRANCH_NAME == 'main') {
                    image.push("${env.BUILD_NUMBER}")
                }
            }
        }
    }
}
```

**How it works:**

1. **`docker.build()`** -- Runs `docker build` and returns an image object. The argument is the image name and tag.

2. **`docker.withRegistry()`** -- Handles authentication. The first argument is the registry URL. The second is a Jenkins credentials ID (configured in Jenkins > Manage Jenkins > Credentials). The plugin automatically runs `docker login` and `docker logout`.

3. **`image.push()`** -- Runs `docker push` with the specified tag. You can call it multiple times to push multiple tags for the same image.

### Setting Up Docker Credentials in Jenkins

1. Go to Jenkins > Manage Jenkins > Credentials > System > Global Credentials
2. Click "Add Credentials"
3. Kind: "Username with password"
4. Username: your registry username (or AWS access key for ECR)
5. Password: your registry password (or AWS secret key)
6. ID: `docker-registry-credentials` (referenced in the Jenkinsfile)

For AWS ECR, use the Amazon ECR plugin which handles token refresh automatically:

```groovy
docker.withRegistry('https://<account>.dkr.ecr.<region>.amazonaws.com', 'ecr:us-east-1:aws-credentials') {
    image.push("${GIT_SHA}")
}
```

---

## Security: Image Scanning with Trivy and Snyk

Container images can contain vulnerable system packages or libraries. Image scanning tools check every installed package against vulnerability databases and report any known CVEs (Common Vulnerabilities and Exposures).

### Why This Matters for This Project

The runtime image installs:
- Alpine Linux packages (ca-certificates, bash, curl, nodejs, npm, git)
- Node.js npm packages (Claude Code CLI and its dependencies)
- Go binaries (with compiled-in Go standard library)

Any of these could have known vulnerabilities. The Node.js dependency tree is especially large and changes frequently.

### Trivy (Open Source)

Trivy is a comprehensive vulnerability scanner from Aqua Security:

```bash
# Install Trivy
curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh

# Scan the built image
trivy image --severity HIGH,CRITICAL claude-terminal-service:latest

# Generate a JSON report
trivy image --format json --output trivy-report.json claude-terminal-service:latest

# Fail if HIGH or CRITICAL vulnerabilities found
trivy image --exit-code 1 --severity HIGH,CRITICAL claude-terminal-service:latest
```

### Snyk (Commercial, Free Tier Available)

Snyk provides deeper analysis with fix suggestions:

```bash
# Authenticate
snyk auth

# Scan the image
snyk container test claude-terminal-service:latest

# Monitor continuously (registers the image for ongoing scanning)
snyk container monitor claude-terminal-service:latest
```

### Jenkins Stage for Image Scanning

```groovy
stage('Image Security Scan') {
    steps {
        // Scan with Trivy
        sh '''
            trivy image \
                --format json \
                --output trivy-report.json \
                --severity HIGH,CRITICAL \
                claude-terminal-service:${GIT_SHA}
        '''

        // Archive the report
        archiveArtifacts artifacts: 'trivy-report.json'

        // Fail the build on CRITICAL vulnerabilities only
        sh '''
            trivy image \
                --exit-code 1 \
                --severity CRITICAL \
                claude-terminal-service:${GIT_SHA}
        '''
    }
}
```

**Policy decisions:**

| Severity | Typical Policy |
|----------|---------------|
| CRITICAL | Block the build. Do not deploy until fixed. |
| HIGH | Warn, but allow deployment. Fix within 1 sprint. |
| MEDIUM | Informational. Track for later remediation. |
| LOW | Ignore in CI. Address during scheduled maintenance. |

### Complete Docker Build and Push Pipeline

Here is a complete Jenkins stage combining build, scan, and push:

```groovy
pipeline {
    agent any

    environment {
        GIT_SHA = sh(script: 'git rev-parse --short HEAD', returnStdout: true).trim()
        IMAGE_NAME = 'claude-terminal-service'
        REGISTRY = 'registry.example.com/midserver'
    }

    stages {
        stage('Docker Build') {
            steps {
                sh """
                    docker build \
                        -t ${IMAGE_NAME}:${GIT_SHA} \
                        -t ${IMAGE_NAME}:latest \
                        .
                """
            }
        }

        stage('Image Security Scan') {
            steps {
                sh "trivy image --exit-code 1 --severity CRITICAL ${IMAGE_NAME}:${GIT_SHA}"
            }
        }

        stage('Push to Registry') {
            when {
                branch 'main'
            }
            steps {
                script {
                    docker.withRegistry("https://${REGISTRY}", 'docker-registry-credentials') {
                        sh "docker tag ${IMAGE_NAME}:${GIT_SHA} ${REGISTRY}/${IMAGE_NAME}:${GIT_SHA}"
                        sh "docker tag ${IMAGE_NAME}:${GIT_SHA} ${REGISTRY}/${IMAGE_NAME}:latest"
                        sh "docker push ${REGISTRY}/${IMAGE_NAME}:${GIT_SHA}"
                        sh "docker push ${REGISTRY}/${IMAGE_NAME}:latest"
                    }
                }
            }
        }

        stage('Integration Test') {
            steps {
                sh 'docker compose up -d --build'
                sh '''
                    for i in $(seq 1 30); do
                        if curl -sf http://localhost:3000/health; then break; fi
                        sleep 5
                    done
                '''
                sh 'curl -sf http://localhost:3000/health'
            }
            post {
                always {
                    sh 'docker compose down -v'
                }
            }
        }
    }
}
```

This pipeline:
1. Builds the Docker image with two tags (SHA and latest)
2. Scans for CRITICAL vulnerabilities and fails if any are found
3. Pushes to the registry only from the main branch
4. Runs integration tests using Docker Compose
5. Always cleans up Docker Compose, even if tests fail
