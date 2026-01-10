#!/bin/bash

set -e

echo "=== LoveBin Production Deployment Setup ==="

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo "Please run as root (use sudo)"
    exit 1
fi

# Update system
echo "Updating system packages..."
apt-get update
apt-get upgrade -y

# Install required packages
echo "Installing required packages..."
apt-get install -y \
    docker.io \
    docker-compose \
    curl \
    wget \
    git \
    ufw \
    fail2ban

# Start and enable Docker
echo "Starting Docker service..."
systemctl start docker
systemctl enable docker

# Configure firewall
echo "Configuring firewall..."
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable

# Configure fail2ban
echo "Configuring fail2ban..."
cat > /etc/fail2ban/jail.local <<EOF
[DEFAULT]
bantime = 3600
findtime = 600
maxretry = 5

[sshd]
enabled = true
port = 22
logpath = /var/log/auth.log
maxretry = 3
EOF

systemctl restart fail2ban
systemctl enable fail2ban

# Create application directory
APP_DIR="/opt/lovebin"
echo "Creating application directory at $APP_DIR..."
mkdir -p $APP_DIR
mkdir -p $APP_DIR/config
mkdir -p $APP_DIR/deploy

# Set permissions
chown -R $SUDO_USER:$SUDO_USER $APP_DIR

echo ""
echo "=== Setup Complete ==="
echo "Next steps:"
echo "1. Copy your application files to $APP_DIR"
echo "2. Copy config/.env to $APP_DIR/config/.env and update with your values"
echo "3. Update DOCKER_IMAGE in config/.env with your Docker Hub path (default: aamira/lovebin)"
echo "4. Run: cd $APP_DIR/deploy && docker-compose -f docker-compose.prod.yml up -d"
echo ""
