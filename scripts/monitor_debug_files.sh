#!/bin/bash

# Script to monitor and copy new webhook debug files
echo "🔍 Webhook Debug File Monitor"
echo "=============================="
echo "Monitoring for new webhook debug files..."
echo "Press Ctrl+C to stop"
echo ""

LAST_COUNT=0

while true; do
    # Get current file count
    CURRENT_COUNT=$(docker exec webhook-processor-prod-webhook-processor-1 find /app -name "raw_webhook_data_*.json" -type f | wc -l)
    
    if [ "$CURRENT_COUNT" -gt "$LAST_COUNT" ]; then
        echo "📄 New debug file(s) detected! (Total: $CURRENT_COUNT)"
        
        # Copy all files with timestamp
        TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
        mkdir -p "debug_files/batch_$TIMESTAMP"
        
        # Get file list and copy them
        docker exec webhook-processor-prod-webhook-processor-1 find /app -name "raw_webhook_data_*.json" -type f | while read file; do
            filename=$(basename "$file")
            docker cp "webhook-processor-prod-webhook-processor-1:$file" "debug_files/batch_$TIMESTAMP/$filename"
            echo "  ✅ Copied: $filename"
        done
        
        echo "  📁 Files saved to: debug_files/batch_$TIMESTAMP/"
        echo ""
        
        LAST_COUNT=$CURRENT_COUNT
    fi
    
    sleep 5
done
