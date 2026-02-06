#!/bin/bash
set -e

echo "==========================================="
echo "ServiceNow MID Server - Container Startup"
echo "==========================================="
echo ""

MID_HOME=${MID_HOME:-/opt/servicenow/mid}

# Check required environment variables
if [ -z "$MID_INSTANCE_URL" ]; then
    echo "ERROR: MID_INSTANCE_URL is not set"
    exit 1
fi

if [ -z "$MID_USERNAME" ]; then
    echo "ERROR: MID_USERNAME is not set"
    exit 1
fi

if [ -z "$MID_PASSWORD" ]; then
    echo "ERROR: MID_PASSWORD is not set"
    exit 1
fi

if [ -z "$MID_SERVER_NAME" ]; then
    echo "ERROR: MID_SERVER_NAME is not set"
    exit 1
fi

echo "Instance URL: $MID_INSTANCE_URL"
echo "MID Server Name: $MID_SERVER_NAME"
echo "Username: $MID_USERNAME"
echo ""

# Configure MID Server
CONFIG_FILE="${MID_HOME}/agent/config.xml"

# Create config.xml if it doesn't exist
if [ ! -f "$CONFIG_FILE" ]; then
    echo "Creating MID Server configuration..."
    cat > "$CONFIG_FILE" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<parameters>
    <parameter name="url" value="${MID_INSTANCE_URL}"/>
    <parameter name="mid.instance.username" value="${MID_USERNAME}"/>
    <parameter name="mid.instance.password" value="${MID_PASSWORD}" encrypt="true"/>
    <parameter name="name" value="${MID_SERVER_NAME}"/>
    <parameter name="mid.proxy.use_proxy" value="${MID_PROXY_HOST:+true}"/>
    <parameter name="mid.proxy.host" value="${MID_PROXY_HOST:-}"/>
    <parameter name="mid.proxy.port" value="${MID_PROXY_PORT:-}"/>
    <parameter name="mid.proxy.username" value="${MID_PROXY_USERNAME:-}"/>
    <parameter name="mid.proxy.password" value="${MID_PROXY_PASSWORD:-}" encrypt="true"/>
</parameters>
EOF
    echo "Configuration file created at $CONFIG_FILE"
fi

# Set permissions
chmod 600 "$CONFIG_FILE"

echo "Starting MID Server..."
echo ""

# Find and execute the MID Server start script
if [ -f "${MID_HOME}/start.sh" ]; then
    exec "${MID_HOME}/start.sh"
elif [ -f "${MID_HOME}/bin/mid.sh" ]; then
    exec "${MID_HOME}/bin/mid.sh" start
elif [ -f "${MID_HOME}/agent/start.sh" ]; then
    exec "${MID_HOME}/agent/start.sh"
else
    echo "ERROR: Could not find MID Server start script"
    ls -la ${MID_HOME}/
    exit 1
fi
