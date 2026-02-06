# Kubernetes Deployment Guide

## Overview

This guide explains how to deploy the Claude Terminal MID Service to Kubernetes with the ServiceNow MID Server running in a separate pod.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│                                                              │
│  Namespace: claude-mid-service                               │
│                                                              │
│  ┌──────────────────────┐    ┌──────────────────────┐      │
│  │  Pod 1: Claude       │    │  Pod 2: ServiceNow   │      │
│  │  Terminal Service    │    │  MID Server          │      │
│  │                      │    │                      │      │
│  │  ┌────────────────┐ │    │  ┌────────────────┐ │      │
│  │  │ HTTP Service   │ │    │  │ MID Server     │ │      │
│  │  │ (port 3000)    │ │    │  │ (ServiceNow)   │ │      │
│  │  └────────────────┘ │    │  └────────────────┘ │      │
│  └──────────────────────┘    └──────────────────────┘      │
│                                                              │
│  ┌──────────────────────┐                                   │
│  │  Pod 3: ECC Queue    │                                   │
│  │  Poller              │                                   │
│  │                      │                                   │
│  │  ┌────────────────┐ │                                   │
│  │  │ ECC Poller     │ │                                   │
│  │  │ (Go binary)    │ │                                   │
│  │  └────────────────┘ │                                   │
│  └──────────────────────┘                                   │
│                                                              │
│  Services:                                                   │
│  • claude-terminal-service (ClusterIP)                      │
│  • servicenow-mid-server (ClusterIP)                        │
│                                                              │
│  Storage:                                                    │
│  • claude-sessions-pvc (10Gi) - Session workspaces          │
│  • mid-server-data-pvc (5Gi) - MID Server data              │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

### 1. Kubernetes Cluster
- Kubernetes 1.20+
- kubectl configured
- Sufficient resources:
  - 2 CPUs
  - 4GB RAM
  - 20GB storage

### 2. Container Registry Access
- Access to push images
- Or use public registry

### 3. ServiceNow MID Server Image
- Official image: `servicenow/mid-server:tokyo` (or your version)
- See: https://docs.servicenow.com/bundle/tokyo-servicenow-platform/page/product/mid-server/concept/mid-server-docker.html

### 4. Build Tools
- Docker installed locally
- make (optional)

## Quick Start

### Step 1: Build and Push Docker Image

```bash
# Build the image
cd /path/to/mid-llm-cli
docker build -f kubernetes/Dockerfile.multi -t your-registry/claude-terminal-service:1.0.0 .

# Push to registry
docker push your-registry/claude-terminal-service:1.0.0
```

### Step 2: Configure Secrets

```bash
# Edit secret.yaml with your credentials
vi kubernetes/secret.yaml

# Update these values:
# - SERVICENOW_API_USER
# - SERVICENOW_API_PASSWORD
# - MID_USERNAME
# - MID_PASSWORD
# - MID_INSTANCE_URL
# - ENCRYPTION_KEY (generate: openssl rand -base64 32)
```

### Step 3: Configure ConfigMap

```bash
# Edit configmap.yaml
vi kubernetes/configmap.yaml

# Update:
# - SERVICENOW_INSTANCE
# - MID_SERVER_NAME
```

### Step 4: Deploy to Kubernetes

```bash
# Apply all manifests
kubectl apply -f kubernetes/

# Or use kustomize
kubectl apply -k kubernetes/
```

### Step 5: Verify Deployment

```bash
# Check pods
kubectl get pods -n claude-mid-service

# Expected output:
# NAME                                      READY   STATUS    RESTARTS   AGE
# claude-terminal-service-xxx               1/1     Running   0          2m
# claude-ecc-poller-xxx                     1/1     Running   0          2m
# servicenow-mid-server-xxx                 1/1     Running   0          2m

# Check services
kubectl get svc -n claude-mid-service

# Check logs
kubectl logs -n claude-mid-service deployment/claude-terminal-service
kubectl logs -n claude-mid-service deployment/claude-ecc-poller
kubectl logs -n claude-mid-service deployment/servicenow-mid-server
```

## Detailed Instructions

### Building the Docker Image

#### Option 1: Using Provided Dockerfile

```bash
# Build multi-binary image
docker build -f kubernetes/Dockerfile.multi \
  -t your-registry/claude-terminal-service:1.0.0 \
  .

# Test locally
docker run -p 3000:3000 \
  -e SERVICENOW_INSTANCE=test.service-now.com \
  -e SERVICENOW_API_USER=test \
  -e SERVICENOW_API_PASSWORD=test \
  your-registry/claude-terminal-service:1.0.0

# Push to registry
docker push your-registry/claude-terminal-service:1.0.0
```

#### Option 2: Using Existing Dockerfile

```bash
# Build using root Dockerfile
docker build -t your-registry/claude-terminal-service:1.0.0 .

# Push
docker push your-registry/claude-terminal-service:1.0.0
```

### Configuring Resources

#### Update Image References

Edit `kubernetes/kustomization.yaml`:

```yaml
images:
  - name: claude-terminal-service
    newName: your-registry/claude-terminal-service  # Your registry
    newTag: "1.0.0"
  - name: servicenow/mid-server
    newName: servicenow/mid-server
    newTag: tokyo  # Your ServiceNow version (tokyo, utah, etc.)
```

#### Configure Storage

Edit `kubernetes/pvc.yaml` if needed:

```yaml
spec:
  resources:
    requests:
      storage: 10Gi  # Adjust size
  storageClassName: standard  # Change to your storage class
```

Check available storage classes:
```bash
kubectl get storageclass
```

#### Configure Resource Limits

Edit deployments if needed:

```yaml
resources:
  requests:
    memory: "256Mi"  # Minimum guaranteed
    cpu: "250m"
  limits:
    memory: "1Gi"    # Maximum allowed
    cpu: "1000m"
```

### ServiceNow MID Server Configuration

#### Getting the Official Image

```bash
# Pull official ServiceNow MID Server image
# Reference: https://docs.servicenow.com/

docker pull servicenow/mid-server:tokyo

# Or specify your version
docker pull servicenow/mid-server:<your-version>
```

#### MID Server Environment Variables

Required variables (set in `secret.yaml`):

- `MID_INSTANCE_URL` - Your ServiceNow instance URL
- `MID_USERNAME` - MID Server user
- `MID_PASSWORD` - MID Server password
- `MID_SERVER_NAME` - Unique name for this MID Server

Optional variables:

- `MID_PROXY_HOST` - Proxy hostname (if behind proxy)
- `MID_PROXY_PORT` - Proxy port
- `MID_PROXY_USERNAME` - Proxy username
- `MID_PROXY_PASSWORD` - Proxy password

### Deployment Strategies

#### Strategy 1: All at Once (Simplest)

```bash
kubectl apply -f kubernetes/
```

#### Strategy 2: Step by Step (Recommended)

```bash
# 1. Create namespace
kubectl apply -f kubernetes/namespace.yaml

# 2. Create storage
kubectl apply -f kubernetes/pvc.yaml

# 3. Create config and secrets
kubectl apply -f kubernetes/configmap.yaml
kubectl apply -f kubernetes/secret.yaml

# 4. Deploy MID Server first
kubectl apply -f kubernetes/deployment-servicenow-mid.yaml
kubectl apply -f kubernetes/service.yaml

# Wait for MID Server to be ready
kubectl wait --for=condition=ready pod \
  -l app=servicenow-mid \
  -n claude-mid-service \
  --timeout=300s

# 5. Deploy Claude Terminal Service
kubectl apply -f kubernetes/deployment-claude-terminal.yaml

# 6. Verify all pods are running
kubectl get pods -n claude-mid-service
```

#### Strategy 3: Using Kustomize

```bash
# Preview what will be applied
kubectl kustomize kubernetes/

# Apply with kustomize
kubectl apply -k kubernetes/

# Or build and pipe
kustomize build kubernetes/ | kubectl apply -f -
```

#### Strategy 4: Using Helm (Advanced)

```bash
# Create Helm chart (see helm/ directory)
helm install claude-mid-service ./kubernetes/helm \
  --namespace claude-mid-service \
  --create-namespace \
  --values kubernetes/helm/values.yaml
```

## Verification

### Health Checks

```bash
# Check HTTP service health
kubectl exec -n claude-mid-service deployment/claude-terminal-service -- \
  curl -s http://localhost:3000/health

# Expected: {"status":"healthy"}
```

### Test Session Creation

```bash
# Port forward to access locally
kubectl port-forward -n claude-mid-service \
  svc/claude-terminal-service 3000:3000 &

# Test API
curl -X POST http://localhost:3000/api/session/create \
  -H "Content-Type: application/json" \
  -d '{"userId":"test","credentials":{"anthropicApiKey":"test-key"}}'
```

### Check MID Server Status

```bash
# Check MID Server logs
kubectl logs -n claude-mid-service deployment/servicenow-mid-server --tail=50

# Should see MID Server connecting to ServiceNow
```

### Verify ECC Queue Processing

```bash
# Check ECC poller logs
kubectl logs -n claude-mid-service deployment/claude-ecc-poller --tail=50

# Should see polling activity
```

## Monitoring

### Pod Status

```bash
# Watch pods
kubectl get pods -n claude-mid-service -w

# Describe pod for details
kubectl describe pod -n claude-mid-service <pod-name>
```

### Resource Usage

```bash
# Check resource consumption
kubectl top pods -n claude-mid-service
kubectl top nodes
```

### Logs

```bash
# Stream logs
kubectl logs -n claude-mid-service -f deployment/claude-terminal-service
kubectl logs -n claude-mid-service -f deployment/claude-ecc-poller
kubectl logs -n claude-mid-service -f deployment/servicenow-mid-server

# Get logs from all containers in namespace
kubectl logs -n claude-mid-service --all-containers=true --tail=100
```

## Scaling

### Horizontal Scaling

```bash
# Scale Claude Terminal Service
kubectl scale deployment claude-terminal-service \
  -n claude-mid-service \
  --replicas=3

# Note: ECC Poller should stay at 1 replica to avoid duplicate processing
# Note: MID Server typically stays at 1 replica per MID Server name
```

### Vertical Scaling

Edit deployment and update resources:

```yaml
resources:
  requests:
    memory: "512Mi"
    cpu: "500m"
  limits:
    memory: "2Gi"
    cpu: "2000m"
```

Apply changes:
```bash
kubectl apply -f kubernetes/deployment-claude-terminal.yaml
```

## Updates

### Rolling Update

```bash
# Build new image
docker build -f kubernetes/Dockerfile.multi \
  -t your-registry/claude-terminal-service:1.1.0 .
docker push your-registry/claude-terminal-service:1.1.0

# Update deployment
kubectl set image deployment/claude-terminal-service \
  claude-terminal-service=your-registry/claude-terminal-service:1.1.0 \
  -n claude-mid-service

# Check rollout status
kubectl rollout status deployment/claude-terminal-service \
  -n claude-mid-service
```

### Rollback

```bash
# View rollout history
kubectl rollout history deployment/claude-terminal-service \
  -n claude-mid-service

# Rollback to previous version
kubectl rollout undo deployment/claude-terminal-service \
  -n claude-mid-service

# Rollback to specific revision
kubectl rollout undo deployment/claude-terminal-service \
  --to-revision=2 \
  -n claude-mid-service
```

## Troubleshooting

### Pods Not Starting

```bash
# Check events
kubectl get events -n claude-mid-service --sort-by='.lastTimestamp'

# Describe pod
kubectl describe pod -n claude-mid-service <pod-name>

# Common issues:
# - ImagePullBackOff: Check image name and registry access
# - CrashLoopBackOff: Check logs for application errors
# - Pending: Check resource availability and PVC binding
```

### Storage Issues

```bash
# Check PVC status
kubectl get pvc -n claude-mid-service

# Describe PVC
kubectl describe pvc claude-sessions-pvc -n claude-mid-service

# If PVC is pending:
# - Check if storage class exists
# - Check if storage class can provision volumes
# - Check cluster has available storage
```

### Network Issues

```bash
# Test pod-to-pod communication
kubectl exec -n claude-mid-service deployment/claude-ecc-poller -- \
  curl -v http://claude-terminal-service:3000/health

# Check service endpoints
kubectl get endpoints -n claude-mid-service
```

### Secret/ConfigMap Issues

```bash
# Verify secret exists
kubectl get secret claude-terminal-secrets -n claude-mid-service -o yaml

# Verify configmap
kubectl get configmap claude-terminal-config -n claude-mid-service -o yaml

# Restart pods to pick up changes
kubectl rollout restart deployment/claude-terminal-service -n claude-mid-service
```

## Security

### RBAC (Role-Based Access Control)

Create ServiceAccount with limited permissions:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: claude-terminal-sa
  namespace: claude-mid-service
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: claude-terminal-role
  namespace: claude-mid-service
rules:
- apiGroups: [""]
  resources: ["pods", "services"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: claude-terminal-rolebinding
  namespace: claude-mid-service
subjects:
- kind: ServiceAccount
  name: claude-terminal-sa
roleRef:
  kind: Role
  name: claude-terminal-role
  apiGroup: rbac.authorization.k8s.io
```

### Network Policies

Restrict pod-to-pod communication:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: claude-terminal-netpol
  namespace: claude-mid-service
spec:
  podSelector:
    matchLabels:
      app: claude-terminal
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: claude-terminal
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: servicenow-mid
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 443  # ServiceNow API
```

### Secrets Management

#### Using Sealed Secrets

```bash
# Install sealed-secrets controller
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.18.0/controller.yaml

# Create sealed secret
kubeseal -f kubernetes/secret.yaml -o yaml > kubernetes/sealed-secret.yaml

# Apply sealed secret
kubectl apply -f kubernetes/sealed-secret.yaml
```

#### Using External Secrets

```bash
# Install external-secrets operator
helm repo add external-secrets https://charts.external-secrets.io
helm install external-secrets external-secrets/external-secrets \
  -n external-secrets-system \
  --create-namespace

# Configure secret store (AWS Secrets Manager, Vault, etc.)
# See: https://external-secrets.io/
```

## Cleanup

### Delete Everything

```bash
# Delete namespace (deletes all resources)
kubectl delete namespace claude-mid-service

# Or delete individual resources
kubectl delete -f kubernetes/
```

### Delete Specific Components

```bash
# Delete deployments only
kubectl delete deployment -n claude-mid-service --all

# Delete services
kubectl delete service -n claude-mid-service --all

# Keep PVCs for data retention
kubectl get pvc -n claude-mid-service
```

## Production Checklist

Before deploying to production:

- [ ] Secrets are properly secured (not hardcoded)
- [ ] Resource limits are configured
- [ ] Health checks are working
- [ ] Persistent storage is configured
- [ ] Monitoring is set up
- [ ] Logging is configured
- [ ] Backup strategy is in place
- [ ] RBAC is configured
- [ ] Network policies are applied
- [ ] Images are scanned for vulnerabilities
- [ ] ServiceNow MID Server is registered and validated
- [ ] End-to-end testing completed

## Next Steps

1. Review [PROJECT_COMPLETE.md](../PROJECT_COMPLETE.md) for overview
2. Test in development cluster first
3. Import ServiceNow update set
4. Verify end-to-end functionality
5. Deploy to production

---

**Version:** 1.0.0
**Last Updated:** 2026-01-24
**Kubernetes:** 1.20+
**ServiceNow:** Tokyo+
