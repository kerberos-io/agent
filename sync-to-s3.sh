#!/bin/bash

# Set script to fail on any error
set -e

# Set working directory (optional, if running via cron)
cd "$(dirname "$0")"

# Define source and destination
LOCAL_DIR="./media/"
S3_BUCKET="s3://kerberos-alus/kerberos-uploads/"

# Sync with AWS S3 (only upload new or changed files)
aws s3 sync "$LOCAL_DIR" "$S3_BUCKET" --storage-class STANDARD --exact-timestamps

# Optional: Log the sync time
echo "$(date): Synced $LOCAL_DIR to $S3_BUCKET" >> sync.log
