#!/bin/bash

# Script to extract Anthropic API key from Claude CLI config

echo "╔════════════════════════════════════════════════════════════╗"
echo "║         Extract Anthropic API Key from Claude CLI         ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Possible config file locations
CONFIG_LOCATIONS=(
    "$HOME/.config/claude/config.json"
    "$HOME/.claude/config.json"
    "$HOME/.anthropic/config.json"
)

# Check each location
for config in "${CONFIG_LOCATIONS[@]}"; do
    if [ -f "$config" ]; then
        echo "✓ Found config file: $config"
        echo ""

        # Try to extract API key using jq (if available)
        if command -v jq &> /dev/null; then
            API_KEY=$(jq -r '.api_key // .apiKey // empty' "$config" 2>/dev/null)
            if [ -n "$API_KEY" ]; then
                echo "API Key: $API_KEY"
                echo ""
                echo "To use in ServiceNow integration:"
                echo "1. Log into ServiceNow"
                echo "2. Navigate to Claude Credential Setup page"
                echo "3. Paste this API key: $API_KEY"
                exit 0
            fi
        else
            # Fallback: show raw config
            echo "Config contents:"
            cat "$config"
            echo ""
            echo "Note: Install 'jq' for automatic extraction: brew install jq"
        fi
    fi
done

echo "⚠ Claude CLI config not found in standard locations"
echo ""
echo "To find your API key manually:"
echo "1. Check: ~/.config/claude/config.json"
echo "2. Or run: claude config show"
echo "3. Or retrieve from: https://console.anthropic.com/settings/keys"
