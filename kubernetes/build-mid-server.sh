#!/bin/bash
set -e

echo "==========================================="
echo "ServiceNow MID Server - Docker Build Script"
echo "==========================================="
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MID_ZIP="${SCRIPT_DIR}/mid-server.zip"
IMAGE_NAME="servicenow-mid-server"
IMAGE_TAG="zurich-patch4-hotfix3"
CLUSTER_NAME="claude-mid-cluster"

# Step 1: Check if MID Server package exists
if [ ! -f "$MID_ZIP" ]; then
    echo "ERROR: MID Server package not found at: $MID_ZIP"
    echo ""
    echo "Please download the MID Server package and save it as:"
    echo "  ${MID_ZIP}"
    echo ""
    echo "Download URL:"
    echo "  https://install.service-now.com/glide/distribution/builds/package/app-signed/mid-linux-container-recipe/2025/12/25/mid-linux-container-recipe.zurich-07-01-2025__patch4-hotfix3-12-23-2025_12-25-2025_1331.linux.x86-64.zip"
    echo ""
    echo "You may need to authenticate to ServiceNow to download the package."
    exit 1
fi

echo "✓ Found MID Server package: $MID_ZIP"
echo "  Size: $(du -h "$MID_ZIP" | cut -f1)"
echo ""

# Step 2: Build Docker image
echo "Building Docker image: ${IMAGE_NAME}:${IMAGE_TAG}"
echo ""

docker build \
    -f "${SCRIPT_DIR}/Dockerfile.mid-server" \
    -t "${IMAGE_NAME}:${IMAGE_TAG}" \
    "${SCRIPT_DIR}"

if [ $? -eq 0 ]; then
    echo ""
    echo "✓ Docker image built successfully"
else
    echo ""
    echo "✗ Docker image build failed"
    exit 1
fi

# Step 3: Load image into kind cluster
echo ""
echo "Loading image into kind cluster: ${CLUSTER_NAME}"
echo ""

kind load docker-image "${IMAGE_NAME}:${IMAGE_TAG}" --name "${CLUSTER_NAME}"

if [ $? -eq 0 ]; then
    echo ""
    echo "✓ Image loaded into kind cluster"
else
    echo ""
    echo "✗ Failed to load image into kind cluster"
    exit 1
fi

# Step 4: Update deployment
echo ""
echo "Updating Kubernetes deployment..."
echo ""

kubectl set image deployment/servicenow-mid-server \
    mid-server="${IMAGE_NAME}:${IMAGE_TAG}" \
    -n claude-mid-service

if [ $? -eq 0 ]; then
    echo ""
    echo "✓ Deployment updated"
else
    echo ""
    echo "✗ Failed to update deployment"
    exit 1
fi

# Step 5: Wait for rollout
echo ""
echo "Waiting for rollout to complete..."
echo ""

kubectl rollout status deployment/servicenow-mid-server -n claude-mid-service --timeout=5m

echo ""
echo "==========================================="
echo "✓ MID Server deployment complete!"
echo "==========================================="
echo ""
echo "Check status:"
echo "  kubectl get pods -n claude-mid-service"
echo ""
echo "View logs:"
echo "  kubectl logs -n claude-mid-service deployment/servicenow-mid-server -f"
echo ""
echo "Verify MID Server registration in ServiceNow:"
echo "  Navigate to: MID Server > Servers"
echo "  Look for: k8s-mid-server-01"
echo ""
