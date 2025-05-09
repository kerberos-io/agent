#!/bin/bash

# Load environment variables from .env if it exists
ENV_FILE="./.env"
if [[ -f "$ENV_FILE" ]]; then
    export $(grep -v '^#' "$ENV_FILE" | xargs)
fi

# Check that the required environment variables are set
if [[ -z "$WEBHOOK_URL" ]]; then
    echo "Error: WEBHOOK_URL not set."
    exit 1
fi

if [[ -z "$AWS_BUCKET_NAME" ]]; then
    echo "Error: AWS_BUCKET_NAME not set."
    exit 1
fi

# Directories and state
WATCH_DIR="./media"
STATE_FILE="./.last_media_file"
LAST_FILE=$(cat "$STATE_FILE" 2>/dev/null)
NEW_FILE=$(ls -t "$WATCH_DIR" | head -n 1)
EXPIRATION_SECONDS=3600  # Signed URL validity (1 hour)

# Only proceed if a new file is detected
if [[ "$NEW_FILE" != "$LAST_FILE" ]]; then
    echo "New motion detected: $NEW_FILE"

    OBJECT_KEY="kerberos-uploads/$NEW_FILE"

    # Generate a signed URL using AWS CLI
    SIGNED_URL=$(aws s3 presign "s3://$AWS_BUCKET_NAME/$OBJECT_KEY" --expires-in $EXPIRATION_SECONDS)

    # Use jq to safely format the Discord JSON payload
    PAYLOAD=$(jq -nc --arg msg "🚨 Motion detected! New file: [$NEW_FILE]($SIGNED_URL)" '{content: $msg}')

    # Send message to Discord webhook
    curl -H "Content-Type: application/json" \
         -X POST \
         -d "$PAYLOAD" \
         "$WEBHOOK_URL"

    # Update the state file
    echo "$NEW_FILE" > "$STATE_FILE"

     # Sync media directory to S3
    echo "Syncing media directory to S3..."
    aws s3 sync "$WATCH_DIR" "s3://$AWS_BUCKET_NAME/kerberos-uploads/" --storage-class STANDARD

else
    echo "No new motion."
fi
