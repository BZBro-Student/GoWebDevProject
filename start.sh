#!/bin/bash
set -e

# Ensure Go bin directory is in PATH (where 'go install' places binaries)
export PATH="$PATH:$(go env GOPATH)/bin"

# ==============================================================================
# Step 1: Check Required Tools
# ==============================================================================
echo "==> [1/4] Checking required tools..."

if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go (v1.20+) and ensure it is in your PATH."
    exit 1
fi

# Check for standalone tailwindcss CLI in PATH or local directory
TAILWIND_CMD=""
if command -v tailwindcss &> /dev/null; then
    TAILWIND_CMD="tailwindcss"
elif [ -x "./tailwindcss" ]; then
    TAILWIND_CMD="./tailwindcss"
else
    echo "Error: Standalone Tailwind CSS CLI is not installed or executable."
    echo "Please install 'tailwindcss' in your PATH or place the executable in the repo root."
    exit 1
fi

echo "All required base tools (go, tailwindcss) are available."

# ==============================================================================
# Step 2: Install Project Dependencies & Dev Tools
# ==============================================================================
echo "==> [2/4] Installing dependencies and building static assets..."

# Download Go module dependencies
go mod download

# Install air for hot reloading if not already installed
if ! command -v air &> /dev/null; then
    echo "Installing 'air' live-reloader..."
    go install github.com/air-verse/air@latest
fi

# Build Tailwind CSS output from input.css using standalone CLI
if [ -f assets/css/input.css ]; then
    echo "Building CSS with standalone Tailwind CLI..."
    mkdir -p static/css
    $TAILWIND_CMD -i assets/css/input.css -o static/css/styles.css --minify
fi

# ==============================================================================
# Step 3: Initialize Database & Directory Setup
# ==============================================================================
echo "==> [3/4] Preparing application environment and data store..."

# Create data directory if it doesn't exist
mkdir -p data

# Ensure database file exists (safe for repeated runs)
if [ ! -f data/app.db ]; then
    touch data/app.db
fi

# ==============================================================================
# Step 4: Start Application & Print URL
# ==============================================================================
PORT="${PORT:-8080}"
APP_URL="http://localhost:${PORT}"

# Kill any existing process using the port to prevent 'address already in use' errors
if lsof -i :${PORT} > /dev/null 2>&1; then
    echo "Freeing port ${PORT}..."
    fuser -k ${PORT}/tcp 2>/dev/null || kill -9 $(lsof -t -i:${PORT}) 2>/dev/null || true
    sleep 1
fi

echo "=================================================="
echo " Application environment ready!"
echo " Server URL: ${APP_URL}"
echo "=================================================="

# Run Air (hot reloading)
exec air