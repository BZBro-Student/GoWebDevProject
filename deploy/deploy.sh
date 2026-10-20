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
DOMAIN="${HOST}.sslip.io"
SSH_CMD="ssh -i $KEY_PATH -o StrictHostKeyChecking=accept-new"

# ==============================================================================
# Step 1: Run Local Tests & Build CSS
# ==============================================================================
echo "==> [1/6] Running local tests..."
go test ./...

# Fixed path to match static/css/input.css
if command -v tailwindcss &> /dev/null && [ -f static/css/input.css ]; then
    echo "Building Tailwind CSS assets..."
    tailwindcss -i static/css/input.css -o static/css/styles.css --minify
fi

# ==============================================================================
# Step 2: Rsync Code to Server
# ==============================================================================
echo "==> [2/6] Syncing project files to $HOST..."
rsync -avz --delete \
    -e "$SSH_CMD" \
    --exclude '.git' \
    --exclude '.gitignore' \
    --exclude '.env' \
    --exclude 'data/*.db' \
    --exclude 'data/*.db-wal' \
    --exclude 'data/*.db-shm' \
    ./ "$REMOTE_USER@$HOST:$REMOTE_APP_DIR/"

# ==============================================================================
# Step 3: Install Remote Dependencies & Build Binary
# ==============================================================================
echo "==> [3/6] Building binary on server..."
$SSH_CMD "$REMOTE_USER@$HOST" "cd $REMOTE_APP_DIR && go mod download && go build -o app ./cmd/app/main.go"

# ==============================================================================
# Step 4: Configure Nginx & Provision SSL Certificate (Idempotent)
# ==============================================================================
echo "==> [4/6] Ensuring Nginx configuration & SSL for $DOMAIN..."

$SSH_CMD "$REMOTE_USER@$HOST" bash -s <<REMOTE_SCRIPT
set -e

# Only write initial HTTP config if Certbot hasn't configured SSL yet
if ! sudo certbot certificates 2>/dev/null | grep -q "$DOMAIN"; then
    sudo tee /etc/nginx/sites-available/gowebdevproject > /dev/null <<EOF
server {
    listen 80;
    server_name $DOMAIN;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \\$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \\$host;
        proxy_cache_bypass \\$http_upgrade;
        proxy_set_header X-Real-IP \\$remote_addr;
        proxy_set_header X-Forwarded-For \\$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \\$scheme;
    }
}
EOF

    sudo ln -sf /etc/nginx/sites-available/gowebdevproject /etc/nginx/sites-enabled/
    sudo rm -f /etc/nginx/sites-enabled/default
    sudo nginx -t
    sudo systemctl reload nginx

    echo "Requesting SSL certificate from Let's Encrypt for $DOMAIN..."
    sudo certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos --register-unsafely-without-email --redirect
else
    echo "SSL certificate already active for $DOMAIN. Reloading Nginx..."
    sudo nginx -t
    sudo systemctl reload nginx
fi
REMOTE_SCRIPT

# ==============================================================================
# Step 5: Restart Application Service
# ==============================================================================
echo "==> [5/6] Restarting systemd service..."
$SSH_CMD "$REMOTE_USER@$HOST" "sudo systemctl restart gowebdevproject.service"

# ==============================================================================
# Step 6: Health Check over HTTPS
# ==============================================================================
echo "==> [6/6] Checking service health on https://$DOMAIN/..."

HEALTH_CHECK_URL="https://$DOMAIN/api/health"
MAX_RETRIES=5
RETRY_DELAY=2
PASSED=false

for i in $(seq 1 $MAX_RETRIES); do
    # Added -L to follow redirects (e.g. 303 -> /login)
    HTTP_STATUS=$(curl -sL -o /dev/null -w "%{http_code}" "$HEALTH_CHECK_URL" || true)
    
    if [ "$HTTP_STATUS" -eq 404 ]; then
        HEALTH_CHECK_URL="https://$DOMAIN/"
        HTTP_STATUS=$(curl -sL -o /dev/null -w "%{http_code}" "$HEALTH_CHECK_URL" || true)
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
