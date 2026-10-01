#!/bin/bash
set -e

# ==============================================================================
# Argument Parsing (-h <PUBLIC-IP> -i <KEY_PATH>)
# ==============================================================================
HOST=""
KEY_PATH=""

while getopts "h:i:" opt; do
  case $opt in
    h) HOST="$OPTARG" ;;
    i) KEY_PATH="$OPTARG" ;;
    *) echo "Usage: $0 -h <PUBLIC-IP> -i <KEY_PATH>" && exit 1 ;;
  esac
done

if [ -z "$HOST" ] || [ -z "$KEY_PATH" ]; then
    echo "Error: Both host (-h) and key path (-i) are required."
    echo "Usage: ./deploy/deploy.sh -h <PUBLIC-IP> -i ~/keys/mykey.pem"
    exit 1
fi

if [ ! -f "$KEY_PATH" ]; then
    echo "Error: Identity file '$KEY_PATH' does not exist."
    exit 1
fi

REMOTE_USER="ubuntu"
REMOTE_APP_DIR="/var/www/gowebdevproject"
SSH_CMD="ssh -i $KEY_PATH -o StrictHostKeyChecking=accept-new"

# ==============================================================================
# Step 1: Run Local Tests & Build CSS
# ==============================================================================
echo "==> [1/5] Running local tests..."
go test ./...

if command -v tailwindcss &> /dev/null && [ -f assets/css/input.css ]; then
    echo "Building Tailwind CSS assets..."
    mkdir -p static/css
    tailwindcss -i assets/css/input.css -o static/css/styles.css --minify
fi

# ==============================================================================
# Step 2: Rsync Code to Server
# ==============================================================================
echo "==> [2/5] Syncing project files to $HOST..."
rsync -avz --delete \
    -e "$SSH_CMD" \
    --exclude '.git' \
    --exclude '.gitignore' \
    --exclude 'data/*.db' \
    ./ "$REMOTE_USER@$HOST:$REMOTE_APP_DIR/"

# ==============================================================================
# Step 3: Install Remote Dependencies & Build Binary
# ==============================================================================
echo "==> [3/5] Building binary on server..."
$SSH_CMD "$REMOTE_USER@$HOST" "cd $REMOTE_APP_DIR && go mod download && go build -o app ./cmd/app/main.go"

# ==============================================================================
# Step 4: Restart Application Service
# ==============================================================================
echo "==> [4/5] Restarting systemd service..."
$SSH_CMD "$REMOTE_USER@$HOST" "sudo systemctl restart gowebdevproject.service"

# ==============================================================================
# Step 5: Health Check
# ==============================================================================
echo "==> [5/5] Checking service health on http://$HOST/..."

HEALTH_CHECK_URL="http://$HOST/api/health"
MAX_RETRIES=5
RETRY_DELAY=2
PASSED=false

for i in $(seq 1 $MAX_RETRIES); do
    HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$HEALTH_CHECK_URL" || true)
    
    # If /api/health isn't defined yet, fall back to checking root route '/'
    if [ "$HTTP_STATUS" -eq 404 ]; then
        HEALTH_CHECK_URL="http://$HOST/"
        HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$HEALTH_CHECK_URL" || true)
    fi

    if [ "$HTTP_STATUS" -eq 200 ]; then
        PASSED=true
        break
    fi

    echo "Attempt $i/$MAX_RETRIES failed (HTTP status: $HTTP_STATUS). Retrying in ${RETRY_DELAY}s..."
    sleep $RETRY_DELAY
done

if [ "$PASSED" = true ]; then
    echo "=================================================="
    echo " Deployment successful!"
    echo " Server verified live at: $HEALTH_CHECK_URL"
    echo "=================================================="
else
    echo "Error: Health check failed! HTTP Status code: $HTTP_STATUS"
    exit 1
fi