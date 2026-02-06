# Chapter 10: Jenkins Setup from Scratch

This chapter walks you through installing Jenkins, configuring the required plugins, and verifying that everything works -- all in the context of building the Claude Terminal MID Service.

---

## Option A: Docker (Recommended for Learning)

This project includes a ready-made Docker Compose file that runs Jenkins locally with all the right settings.

### Step 1: Start Jenkins

```bash
cd /path/to/mid-llm-cli/jenkins

docker compose -f docker-compose.jenkins.yml up -d
```

This pulls the `jenkins/jenkins:lts-jdk17` image and starts a container named `jenkins-local` on port 8080. The first boot takes 2-3 minutes because Jenkins initializes its home directory and installs bundled plugins.

### Step 2: Get the Initial Admin Password

Jenkins generates a one-time password on first boot. Retrieve it:

```bash
docker exec jenkins-local cat /var/jenkins_home/secrets/initialAdminPassword
```

Copy the output. It looks like `a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6`.

### Step 3: Open the Jenkins UI

Open your browser to:

```
http://localhost:8080
```

Paste the admin password from the previous step into the "Unlock Jenkins" field and click **Continue**.

### Step 4: Install Plugins

You will see two options:

- **Install suggested plugins** -- This installs a broad set of common plugins. It is the safe choice.
- **Select plugins to install** -- This lets you pick exactly what you want.

Choose **Install suggested plugins** for now. We will install the project-specific plugins in the next step.

Wait for the installation to complete. This takes 3-5 minutes depending on your connection.

### Step 5: Create the Admin User

Fill in the form:

| Field | Value |
|-------|-------|
| Username | admin |
| Password | (your choice) |
| Full name | Admin |
| Email | admin@localhost |

Click **Save and Continue**, then **Save and Finish**, then **Start using Jenkins**.

### Step 6: Install Project-Specific Plugins

The Claude Terminal MID Service pipeline requires plugins beyond the defaults. Navigate to:

**Manage Jenkins > Plugins > Available plugins**

Search for and install each of the following:

| Plugin | What It Does for This Project |
|--------|-------------------------------|
| **Pipeline** (workflow-aggregator) | Enables declarative `Jenkinsfile` syntax. Without this, Jenkins cannot parse our `Jenkinsfile` at all. |
| **Docker Pipeline** (docker-workflow) | Lets pipeline stages run inside Docker containers. Our pipeline uses `agent { docker { image 'golang:1.24-alpine' } }`. |
| **Go** (golang) | Adds a Go tool installation option in Global Tool Configuration. Lets you auto-install Go 1.24 on agents. |
| **Credentials Binding** | Maps Jenkins credentials into environment variables inside pipeline stages. We use this for Docker registry and ServiceNow secrets. |
| **Git** | Enables `checkout scm` to clone Git repositories. Required for any Git-based project. |
| **GitHub** | Adds GitHub webhook support and status checks. Enables automatic build triggers on push events. |
| **Blue Ocean** | Modern UI for visualizing pipeline stages. Shows a clear diagram of which stages passed/failed. |
| **JUnit** | Parses JUnit XML test reports and shows test results in the Jenkins UI. Our pipeline generates `test-results.xml` via `gotestsum`. |
| **Cobertura** | Parses coverage reports. Alternative to the HTML publisher for coverage data. |
| **Warnings Next Generation** (warnings-ng) | Aggregates warnings from linters and static analysis tools. Can parse `golangci-lint` output. |
| **Timestamper** | Adds timestamps to every line of console output. Essential for diagnosing timing issues in builds. |
| **Pipeline Utility Steps** | Adds utility steps like `readJSON`, `writeJSON`, `findFiles`. Useful for parsing Trivy reports. |
| **Slack Notification** | Sends build success/failure messages to a Slack channel. Our `post` blocks have commented-out `slackSend` calls ready to enable. |
| **Docker Commons** | Shared Docker utilities used by Docker Pipeline. Usually installed as a dependency. |
| **HTML Publisher** | Publishes HTML reports (like `coverage.html`) as links in the Jenkins build page. |

After selecting all plugins, click **Install without restart**. When installation completes, click **Restart Jenkins** if prompted.

### Step 7: Verify Plugin Installation

Navigate to **Manage Jenkins > Plugins > Installed plugins** and confirm that all 15 plugins from the list above are present.

---

## Option B: Native Install (macOS / Linux)

If you prefer to install Jenkins directly on your machine without Docker.

### macOS (Homebrew)

```bash
# Install Jenkins LTS
brew install jenkins-lts

# Start Jenkins as a background service
brew services start jenkins-lts

# Open the UI
open http://localhost:8080
```

The initial admin password is at:

```
/usr/local/var/jenkins/home/secrets/initialAdminPassword
```

### Ubuntu / Debian

```bash
# Add the Jenkins repository key
curl -fsSL https://pkg.jenkins.io/debian-stable/jenkins.io-2023.key | sudo tee \
    /usr/share/keyrings/jenkins-keyring.asc > /dev/null

# Add the repository
echo deb [signed-by=/usr/share/keyrings/jenkins-keyring.asc] \
    https://pkg.jenkins.io/debian-stable binary/ | sudo tee \
    /etc/apt/sources.list.d/jenkins.list > /dev/null

# Install Jenkins
sudo apt-get update
sudo apt-get install -y jenkins

# Start the service
sudo systemctl enable --now jenkins

# Get the initial password
sudo cat /var/lib/jenkins/secrets/initialAdminPassword
```

Then follow Steps 3-7 from Option A above.

---

## Configure Go Tool Installation

Jenkins needs to know where Go is installed so that pipeline stages can use `go build`, `go test`, etc.

### Method 1: Jenkins Global Tool Configuration (Recommended)

1. Navigate to **Manage Jenkins > Tools**
2. Scroll to the **Go** section (visible after installing the Go plugin)
3. Click **Add Go**
4. Configure:

| Field | Value |
|-------|-------|
| Name | `go-1.24` |
| Install automatically | Checked |
| Version | `Go 1.24.1` |

5. Click **Save**

Now pipeline stages can reference this installation:

```groovy
tools {
    go 'go-1.24'
}
```

However, our `Jenkinsfile` uses a Docker agent (`golang:1.24-alpine`) instead, which includes Go pre-installed. The tool configuration is only needed if you run builds on native Jenkins agents.

### Method 2: Docker Agent (What Our Pipeline Uses)

Our pipeline sidesteps the Go tool configuration entirely by running inside a Go Docker container:

```groovy
agent {
    docker {
        image 'golang:1.24-alpine'
        args '-v go-mod-cache:/go/pkg/mod'
    }
}
```

This is the approach used in the project's `Jenkinsfile`. It guarantees the exact Go version regardless of what is installed on the Jenkins controller or agents.

---

## Configure Docker

For our pipeline to build Docker images (the "Docker Build" stage), Jenkins needs access to Docker.

### If Using the Docker Compose Setup (Option A)

The `docker-compose.jenkins.yml` file already mounts the Docker socket:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

And the entrypoint installs the Docker CLI inside the Jenkins container. No additional configuration is needed.

### If Using a Native Jenkins Install

1. Install Docker on the Jenkins host
2. Add the `jenkins` user to the `docker` group:

```bash
sudo usermod -aG docker jenkins
sudo systemctl restart jenkins
```

3. Verify from inside a Jenkins pipeline:

```groovy
pipeline {
    agent any
    stages {
        stage('Docker Test') {
            steps {
                sh 'docker version'
            }
        }
    }
}
```

---

## Verify Setup: Hello World Pipeline Test

Before running the real `Jenkinsfile`, create a quick test to confirm everything works.

### Step 1: Create a Test Job

1. From the Jenkins dashboard, click **New Item**
2. Enter the name: `hello-world-test`
3. Select **Pipeline**
4. Click **OK**

### Step 2: Enter the Test Pipeline

Scroll to the **Pipeline** section and paste:

```groovy
pipeline {
    agent {
        docker {
            image 'golang:1.24-alpine'
        }
    }
    stages {
        stage('Verify Go') {
            steps {
                sh 'go version'
            }
        }
        stage('Verify Docker') {
            steps {
                sh 'docker version || echo "Docker not available inside this agent"'
            }
        }
        stage('Verify Git') {
            steps {
                sh 'apk add --no-cache git && git --version'
            }
        }
        stage('Hello World') {
            steps {
                echo 'Jenkins is ready for the Claude Terminal MID Service pipeline.'
            }
        }
    }
}
```

### Step 3: Run It

Click **Save**, then click **Build Now** in the left sidebar.

### Step 4: Check the Results

Click on the build number (e.g., **#1**) in the build history, then click **Console Output**.

You should see:

```
go version go1.24.1 linux/amd64
```

And the final message:

```
Jenkins is ready for the Claude Terminal MID Service pipeline.
```

If all four stages pass, your Jenkins installation is ready. Proceed to [Chapter 11: Your First Jenkinsfile](11-jenkinsfile-walkthrough.md) to understand the real pipeline.

---

## Troubleshooting

### "Docker not found" Inside Pipeline

The Docker CLI is not installed inside the `golang:1.24-alpine` container. For stages that need Docker, either:
- Mount the host Docker socket (already done in `docker-compose.jenkins.yml`)
- Install Docker CLI in a build step: `apk add --no-cache docker`
- Use a different agent image that includes Docker

### "Permission denied" on Docker Socket

```bash
# On the Jenkins host:
sudo chmod 666 /var/run/docker.sock

# Or add jenkins to the docker group:
sudo usermod -aG docker jenkins
```

### Plugin Installation Fails

If you are behind a corporate proxy, configure the proxy in **Manage Jenkins > Plugins > Advanced Settings**. Enter your HTTP proxy host and port.

### Jenkins Runs Out of Memory

The default Java heap is 256 MB. Increase it by setting `JAVA_OPTS` in the Docker Compose file:

```yaml
environment:
  JAVA_OPTS: "-Xmx2g -Xms512m"
```

Or for a native install, edit `/etc/default/jenkins` (Linux) or the Homebrew plist (macOS).

---

## What You Have Now

After completing this chapter:

- Jenkins is running (Docker or native)
- All 15 required plugins are installed
- Go tool is configured (or Docker agent is verified)
- Docker access is confirmed
- A Hello World pipeline ran successfully

Next: [Chapter 11: Your First Jenkinsfile -- Line by Line](11-jenkinsfile-walkthrough.md)
