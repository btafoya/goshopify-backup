#!/bin/bash
# Installation script for goshopify-backup

set -e

INSTALL_DIR="/opt/goshopify-backup"
SERVICE_NAME="goshopify-backup"
BINARY_NAME="goshopify-backup"

echo "=== Shopify Backup Tool Installation ==="

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "This script must be run as root"
    exit 1
fi

# Create user
if ! id "$SERVICE_NAME" &>/dev/null; then
    echo "Creating user $SERVICE_NAME..."
    useradd -r -s /bin/false -d "$INSTALL_DIR" "$SERVICE_NAME" || true
fi

# Create installation directory
echo "Installing to $INSTALL_DIR..."
mkdir -p "$INSTALL_DIR/backups"

# Copy binary
echo "Copying binary..."
if [ -f "./goshopify-backup" ]; then
    cp ./goshopify-backup "$INSTALL_DIR/$BINARY_NAME"
elif [ -f "./bin/goshopify-backup" ]; then
    cp ./bin/goshopify-backup "$INSTALL_DIR/$BINARY_NAME"
else
    echo "Error: Binary not found. Build first with 'make build'"
    exit 1
fi

chmod +x "$INSTALL_DIR/$BINARY_NAME"

# Set permissions
chown -R "${SERVICE_NAME}:${SERVICE_NAME}" "$INSTALL_DIR"
chmod 750 "$INSTALL_DIR"

# Check for .env file
if [ ! -f "$INSTALL_DIR/.env" ]; then
    if [ -f "./.env" ]; then
        echo "Copying .env file..."
        cp .env "$INSTALL_DIR/.env"
        chmod 600 "$INSTALL_DIR/.env"
        chown "${SERVICE_NAME}:${SERVICE_NAME}" "$INSTALL_DIR/.env"
    else
        echo "Warning: No .env file found. Please create one at $INSTALL_DIR/.env"
        echo "Example .env file:"
        echo "SHOPIFY_STORE=https://your-store.myshopify.com"
        echo "SHOPIFY_ACCESS_TOKEN=your_access_token"
        echo "BACKUP_DIR=/opt/goshopify-backup/backups"
        echo "RETENTION_DAYS=30"
    fi
fi

# Install systemd service
echo "Installing systemd service..."
cp deploy/goshopify-backup.service /etc/systemd/system/
cp deploy/goshopify-backup.timer /etc/systemd/system/

systemctl daemon-reload
systemctl enable "$SERVICE_NAME.timer"
systemctl start "$SERVICE_NAME.timer"

echo ""
echo "=== Installation Complete ==="
echo "Binary installed at: $INSTALL_DIR/$BINARY_NAME"
echo "Config file location: $INSTALL_DIR/.env"
echo "Backup directory: $INSTALL_DIR/backups"
echo ""
echo "Systemd commands:"
echo "  systemctl status $SERVICE_NAME.service  - Check backup status"
echo "  systemctl start $SERVICE_NAME.service    - Run backup manually"
echo "  systemctl stop $SERVICE_NAME.timer     - Disable scheduled backups"
echo "  systemctl start $SERVICE_NAME.timer     - Enable scheduled backups"
echo "  journalctl -u $SERVICE_NAME              - View logs"
echo ""
echo "To test the backup manually:"
echo "  sudo -u $SERVICE_NAME $INSTALL_DIR/$BINARY_NAME --force"