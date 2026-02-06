# Chapter 12: Jenkins Credentials & Secrets

This chapter explains how to manage secrets in Jenkins for the Claude Terminal MID Service. You will learn why hardcoding secrets is dangerous, how Jenkins credentials work, and how to configure every credential this project needs.

---

## Why Never Hardcode Secrets

Look at the project's `.env.example` file:

```bash
SERVICENOW_INSTANCE=your-instance.service-now.com
SERVICENOW_API_USER=integration_user
SERVICENOW_API_PASSWORD=your_password
ENCRYPTION_KEY=generate_a_strong_random_key_here
API_AUTH_TOKEN=generate_a_strong_random_token_here
```

This file is a template. It contains placeholder values. But if someone copies this to `.env` and fills in real values, those secrets exist in a plain-text file on disk.

Now look at the project's `docker-compose.yml`:

```yaml
servicenow-mid-server:
    environment:
      MID_INSTANCE_URL: "https://empsuren.service-now.com"
      MID_INSTANCE_USERNAME: "mid.user"
      MID_INSTANCE_PASSWORD: "_7PJmFo!ifiuc6X9cVV-"
```

This is a real password committed to version control. Anyone with access to the repository can see it. If this repository were ever made public (accidentally or intentionally), the password would be exposed.

**The rule**: Secrets must never appear in:
- Source code or configuration files committed to Git
- Jenkins pipeline definitions (the `Jenkinsfile`)
- Console output (Jenkins masks credential values, but only if they come from the credential store)
- Docker image labels or build arguments

Instead, secrets are stored in Jenkins' built-in credential store, which encrypts them at rest and provides controlled access.

---

## Jenkins Credential Types

Jenkins supports several credential types. Here are the ones relevant to this project:

### Username with Password

Stores a username and a password as a pair. Used for services that require basic authentication.

**Example use case**: Docker registry login (`docker login -u user -p pass`), ServiceNow API calls.

In a Jenkinsfile, you access both values:

```groovy
withCredentials([
    usernamePassword(
        credentialsId: 'docker-registry-creds',
        usernameVariable: 'DOCKER_USER',
        passwordVariable: 'DOCKER_PASS'
    )
]) {
    sh 'echo "$DOCKER_PASS" | docker login -u "$DOCKER_USER" --password-stdin'
}
```

### Secret Text

Stores a single string value (a token, a key, an API key).

**Example use case**: The `ENCRYPTION_KEY` (a 64-character hex string) and `API_AUTH_TOKEN` (a bearer token).

In a Jenkinsfile:

```groovy
withCredentials([
    string(credentialsId: 'encryption-key', variable: 'ENCRYPTION_KEY')
]) {
    sh 'echo "Key length: ${#ENCRYPTION_KEY}"'  // Safe: only prints the length
}
```

### Secret File

Stores an entire file (e.g., a JSON key file, a `.env` file, a certificate).

**Example use case**: A GCP service account JSON file for deploying to Google Cloud, or a complete `.env` file for the service.

In a Jenkinsfile:

```groovy
withCredentials([
    file(credentialsId: 'env-file', variable: 'ENV_FILE')
]) {
    sh 'cp "$ENV_FILE" .env'
}
```

### SSH Username with Private Key

Stores an SSH private key with an optional passphrase. Used for Git operations over SSH.

**Example use case**: Cloning a private Git repository, deploying via SSH to a server.

---

## How to Add Credentials in Jenkins UI

### Step 1: Navigate to Credentials

From the Jenkins dashboard:

1. Click **Manage Jenkins** (left sidebar)
2. Click **Credentials** (under Security)
3. Click **(global)** under "Stores scoped to Jenkins"
4. Click **Add Credentials** (left sidebar)

### Step 2: Fill in the Form

The form fields depend on the credential type. Here is the general layout:

| Field | Description |
|-------|-------------|
| **Kind** | The credential type (Username with password, Secret text, etc.) |
| **Scope** | `Global` (available to all jobs) or `System` (only for Jenkins system operations) |
| **ID** | A unique identifier you reference in the Jenkinsfile (e.g., `docker-registry-creds`) |
| **Description** | A human-readable label (e.g., "Docker Hub credentials for CI builds") |

### Step 3: Save

Click **Create**. The credential is now encrypted and stored in Jenkins' home directory.

---

## Credentials Needed for This Project

### 1. Docker Registry Credentials

| Field | Value |
|-------|-------|
| Kind | Username with password |
| ID | `docker-registry-creds` |
| Description | Docker registry credentials for pushing claude-terminal-service images |
| Username | Your Docker Hub username (or ECR/GCR service account) |
| Password | Your Docker Hub password (or access token) |

**Where it is used in the Jenkinsfile:**

```groovy
// Stage: Docker Push
withCredentials([
    usernamePassword(
        credentialsId: 'docker-registry-creds',
        usernameVariable: 'DOCKER_USER',
        passwordVariable: 'DOCKER_PASS'
    )
]) {
    sh 'echo "$DOCKER_PASS" | docker login -u "$DOCKER_USER" --password-stdin ${REGISTRY}'
}
```

**How to create the value:**
- For Docker Hub: Use your Docker Hub username and an access token (Settings > Security > New Access Token).
- For AWS ECR: Use `AWS` as the username and the output of `aws ecr get-login-password` as the password.
- For GCR: Use `_json_key` as the username and the service account JSON file contents as the password.

### 2. Docker Registry URL

| Field | Value |
|-------|-------|
| Kind | Secret text |
| ID | `docker-registry-url` |
| Description | Docker registry base URL |
| Secret | `docker.io/yourorg` (or `123456789.dkr.ecr.us-east-1.amazonaws.com`) |

**Where it is used:**

```groovy
environment {
    REGISTRY = credentials('docker-registry-url')
}
```

The `credentials()` function in the `environment` block reads a secret text credential and exposes it as an environment variable.

### 3. ServiceNow API Credentials

| Field | Value |
|-------|-------|
| Kind | Username with password |
| ID | `servicenow-api-creds` |
| Description | ServiceNow integration user for ECC Queue operations |
| Username | The ServiceNow API user (e.g., `integration_user`) |
| Password | The ServiceNow API password |

**Where it is used:**

```groovy
// Stage: Deploy Staging
withCredentials([
    usernamePassword(
        credentialsId: 'servicenow-api-creds',
        usernameVariable: 'SN_USER',
        passwordVariable: 'SN_PASS'
    )
]) {
    // Pass to the deployed service configuration
}
```

**Important**: These credentials are for the deployed service to communicate with ServiceNow, not for Jenkins itself. They are injected during deployment so the running `ecc-poller` and `claude-terminal-service` can authenticate to the ServiceNow instance.

### 4. Encryption Key

| Field | Value |
|-------|-------|
| Kind | Secret text |
| ID | `encryption-key` |
| Description | AES-256-GCM encryption key for credential protection at rest |
| Secret | A 64-character hexadecimal string |

**How to generate the value:**

```bash
openssl rand -hex 32
```

This produces 32 random bytes encoded as 64 hex characters (e.g., `a1b2c3d4...`). This key is used by the `internal/crypto/crypto.go` module to encrypt Anthropic API keys stored in user sessions.

**Where it is used:**

```groovy
// Stage: Deploy Staging
withCredentials([
    string(credentialsId: 'encryption-key', variable: 'ENCRYPTION_KEY')
]) {
    // Pass to the deployed service as an environment variable
}
```

### 5. API Auth Token

| Field | Value |
|-------|-------|
| Kind | Secret text |
| ID | `api-auth-token` |
| Description | Bearer token for authenticating to the claude-terminal-service API |
| Secret | A strong random string |

**How to generate the value:**

```bash
openssl rand -base64 32
```

This is the token that clients (the ECC Poller, the MID Server Proxy, or integration tests) include in the `Authorization: Bearer <token>` header when calling the HTTP API.

**Where it is used:**

```groovy
withCredentials([
    string(credentialsId: 'api-auth-token', variable: 'API_AUTH_TOKEN')
]) {
    // Available in the deployment environment
}
```

---

## How `credentials()` Binding Works in the Jenkinsfile

There are two ways to use credentials in a declarative pipeline:

### Method 1: `credentials()` in the `environment` Block

```groovy
environment {
    REGISTRY = credentials('docker-registry-url')
}
```

This works for **secret text** credentials. The value is available as `${REGISTRY}` in every stage. Jenkins automatically masks the value in console output.

For **username/password** credentials, this creates three variables:
- `REGISTRY` -- the string `username:password`
- `REGISTRY_USR` -- just the username
- `REGISTRY_PSW` -- just the password

### Method 2: `withCredentials()` in a Stage

```groovy
withCredentials([
    usernamePassword(credentialsId: 'docker-registry-creds',
                     usernameVariable: 'DOCKER_USER',
                     passwordVariable: 'DOCKER_PASS')
]) {
    sh 'echo "$DOCKER_PASS" | docker login ...'
}
```

This is more explicit. The variables only exist within the `withCredentials` block. This is the preferred method because:
1. The variable names are clear and intentional
2. The scope is limited to where the credentials are needed
3. It works with all credential types

### How Masking Works

When Jenkins encounters a credential value in console output, it replaces it with `****`. For example:

```
+ echo "$DOCKER_PASS" | docker login -u myuser --password-stdin docker.io/myorg
WARNING! Your password will be stored unencrypted in /home/jenkins/.docker/config.json.
Login Succeeded
```

The actual password never appears. Jenkins compares every output line against all credential values used in the current build and masks matches.

**Caveat**: If a credential value is very short (e.g., `abc`), masking might produce false positives by masking `abc` anywhere it appears in the output. Use strong, unique credential values.

---

## Credential Scoping

Jenkins credentials can be scoped to different levels:

### Global Scope

Available to every job on the Jenkins instance. This is the default and works for most cases.

**When to use**: When multiple jobs need the same credentials (e.g., all jobs push to the same Docker registry).

### Folder Scope

Available only to jobs inside a specific Jenkins folder. If you organize jobs as:

```
Jenkins
  mid-llm-cli/        <-- folder
    main-pipeline      <-- job
    nightly-pipeline   <-- job
    pr-pipeline        <-- job
```

You can create credentials at the `mid-llm-cli` folder level. Only the three jobs inside that folder can access them.

**When to use**: When different teams share a Jenkins instance and should not access each other's secrets.

**How to create**: Navigate to the folder in Jenkins, click **Credentials** in the left sidebar, then add credentials as usual.

### Pipeline-Level (via Credential Domains)

You can restrict credentials to specific domain patterns. For example, a Docker Hub credential can be restricted to the domain `docker.io`. This prevents accidental use against the wrong registry.

---

## Rotating Secrets

### When to Rotate

| Credential | Rotation Frequency | Trigger |
|------------|-------------------|---------|
| Docker registry password | Every 90 days | Calendar reminder or automated policy |
| ServiceNow API password | Every 90 days | ServiceNow password policy |
| Encryption key | On key compromise | Requires re-encrypting all stored sessions |
| API auth token | Every 90 days or on compromise | Requires updating all clients (ECC Poller, MID Proxy) |

### How to Rotate

1. **Generate the new credential value** (new password, new key, etc.)
2. **Update the Jenkins credential**:
   - Navigate to **Manage Jenkins > Credentials**
   - Click on the credential ID
   - Click **Update**
   - Enter the new value
   - Click **Save**
3. **Trigger a new deployment** so the running services pick up the new value
4. **Verify** the service still works with the new credential

### Zero-Downtime Rotation for the Encryption Key

The encryption key (`ENCRYPTION_KEY`) is special because it encrypts data at rest. To rotate it without losing existing encrypted sessions:

1. Add the new key as a separate credential (`encryption-key-v2`)
2. Update the service to try decryption with the new key first, then fall back to the old key
3. Re-encrypt all existing sessions with the new key
4. Remove the old key credential

This project does not currently implement key rotation logic in `internal/crypto/crypto.go`. For now, rotating the encryption key will invalidate all existing encrypted credentials in active sessions (users will need to re-enter their Anthropic API key).

---

## Security Best Practices

1. **Use "Secret text" for single values, "Username with password" for pairs.** Do not store a password as secret text and a username as a separate secret text -- keep them together.

2. **Never print credentials in pipeline steps.** Even `echo "Token is: $TOKEN"` will show `Token is: ****`, but it is a bad habit. If someone adds a `set -x` to a shell block, the raw value might leak.

3. **Use `--password-stdin` for Docker login.** Never use `docker login -p $PASS` because the password appears in `ps` output.

4. **Scope credentials as narrowly as possible.** If only one pipeline needs a credential, put it in a folder scope, not global.

5. **Audit credential usage.** Jenkins tracks which builds used which credentials. Review this periodically under **Manage Jenkins > Credentials > (credential) > Track Usage**.

6. **Back up Jenkins credentials.** The credentials are stored encrypted in `$JENKINS_HOME/credentials.xml` and the encryption key is in `$JENKINS_HOME/secrets/`. Back up both files. If you lose the secrets directory, all credentials are irrecoverable.

---

## Summary

| Credential ID | Type | Purpose |
|---------------|------|---------|
| `docker-registry-creds` | Username/Password | Push images to Docker Hub/ECR/GCR |
| `docker-registry-url` | Secret text | Registry URL (e.g., `docker.io/yourorg`) |
| `servicenow-api-creds` | Username/Password | ServiceNow API authentication for deployments |
| `encryption-key` | Secret text | AES-256-GCM key for credential encryption |
| `api-auth-token` | Secret text | Bearer token for the HTTP API |

All five must be created in Jenkins before the full pipeline can run. The pipeline will still build and test without them, but the Docker Push and Deploy stages will fail.

---

Next: [Chapter 13: Webhooks & Triggers](13-webhooks-and-triggers.md)
