#!/bin/bash

# Exit on any error
set -e

BACKUP_PATH="/opt/cmon_backup"
TARGET_PATH="/opt/cmon"

# Check for restore flag
if [ "$1" == "--restore" ] || [ "$1" == "-r" ]; then
    echo "⏪ Restoring from backup ($BACKUP_PATH)..."
    if [ ! -f "$BACKUP_PATH" ]; then
        echo "❌ No backup found at $BACKUP_PATH"
        exit 1
    fi
    
    echo "🛑 Stopping the cmon service..."
    sudo systemctl stop cmon
    
    echo "📂 Restoring binary..."
    sudo cp "$BACKUP_PATH" "$TARGET_PATH"
    
    echo "🚀 Restarting the cmon service..."
    sudo systemctl restart cmon
    
    echo "✅ Restore completed successfully!"
    exit 0
fi

# Normal deployment process
echo "🔄 Pulling latest changes from GitHub..."
cd ~/app/cmon
git pull

echo "🔨 Building the cmon binary..."
go build -o cmon

echo "🛑 Stopping the cmon service..."
cd ~
sudo systemctl stop cmon

if [ -f "$TARGET_PATH" ]; then
    echo "📦 Backing up current working binary to $BACKUP_PATH..."
    sudo cp "$TARGET_PATH" "$BACKUP_PATH"
fi

echo "📂 Copying new binary to $TARGET_PATH..."
sudo cp app/cmon/cmon "$TARGET_PATH"

echo "🚀 Restarting the cmon service..."
sudo systemctl restart cmon

echo "✅ Deployment completed successfully!"
