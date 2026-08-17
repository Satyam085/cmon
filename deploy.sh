#!/bin/bash

# Exit on any error
set -e

BACKUP_PATH="/opt/cmon_backup"
TARGET_PATH="/opt/cmon"
DB_BACKUP_LATEST="/opt/cmon_db_backup.db"
BACKUP_DIR="/opt/cmon_backups"

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
    
    # Optional DB restore if requested with --db flag
    if [ "$2" == "--db" ] && [ -f "$DB_BACKUP_LATEST" ]; then
        echo "🗄️ Restoring database from $DB_BACKUP_LATEST..."
        for db_target in "/opt/cmon.db" "$HOME/cmon.db" "$HOME/app/cmon/cmon.db"; do
            if [ -f "$db_target" ]; then
                sudo cp "$DB_BACKUP_LATEST" "$db_target"
                echo "✓ Database restored to $db_target"
                break
            fi
        done
    fi
    
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

# Ensure backup directory exists
sudo mkdir -p "$BACKUP_DIR"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

if [ -f "$TARGET_PATH" ]; then
    echo "📦 Backing up current working binary to $BACKUP_PATH and $BACKUP_DIR/cmon_$TIMESTAMP..."
    sudo cp "$TARGET_PATH" "$BACKUP_PATH"
    sudo cp "$TARGET_PATH" "$BACKUP_DIR/cmon_$TIMESTAMP"
fi

# SQLite DB Backup (performed while service is stopped so SQLite is cleanly checkpointed)
for db_candidate in "/opt/cmon.db" "$HOME/cmon.db" "$HOME/app/cmon/cmon.db"; do
    if [ -f "$db_candidate" ]; then
        echo "🗄️ Backing up database ($db_candidate)..."
        sudo cp "$db_candidate" "$DB_BACKUP_LATEST"
        sudo cp "$db_candidate" "$BACKUP_DIR/cmon_db_${TIMESTAMP}.db"
        # Also copy WAL and SHM files if present
        [ -f "${db_candidate}-wal" ] && sudo cp "${db_candidate}-wal" "$BACKUP_DIR/cmon_db_${TIMESTAMP}.db-wal" || true
        [ -f "${db_candidate}-shm" ] && sudo cp "${db_candidate}-shm" "$BACKUP_DIR/cmon_db_${TIMESTAMP}.db-shm" || true
        echo "✓ Database safely backed up to $BACKUP_DIR/cmon_db_${TIMESTAMP}.db and $DB_BACKUP_LATEST"
        break
    fi
done

echo "📂 Copying new binary to $TARGET_PATH..."
sudo cp app/cmon/cmon "$TARGET_PATH"

echo "🚀 Restarting the cmon service..."
sudo systemctl restart cmon

echo "✅ Deployment completed successfully!"
