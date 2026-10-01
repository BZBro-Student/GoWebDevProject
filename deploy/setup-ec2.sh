#!/bin/bash
set -e

# Non-interactive mode for apt
export DEBIAN_FRONTEND=noninteractive

echo "==> [1/4] Updating package index and installing Go runtime & Nginx..."
apt-get update -y
apt-get install -y golang-go nginx curl git build-essential

echo "==> [2/4] Creating application directory..."
APP_DIR="/var/www/gowebdevproject"
mkdir -p "$APP_DIR"
mkdir -p "$APP_DIR/data"
chown -R ubuntu:ubuntu "$APP_DIR"

echo "==> [3/4] Setting up Nginx reverse proxy..."
cp deploy/nginx-gowebdevproject.conf /etc/nginx/sites-available/gowebdevproject.conf
ln -sf /etc/nginx/sites-available/gowebdevproject.conf /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default

nginx -t
systemctl restart nginx

echo "==> [4/4] Installing and enabling systemd service..."
cp deploy/gowebdevproject.service /etc/systemd/system/gowebdevproject.service
systemctl daemon-reload
systemctl enable gowebdevproject.service

echo "=================================================="
echo " EC2 Environment Setup Complete!"
echo " App Directory: $APP_DIR"
echo " Service Installed: gowebdevproject.service"
echo "=================================================="