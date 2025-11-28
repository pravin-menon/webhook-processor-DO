#!/bin/bash

echo "🔍 Quick Debug File Check"
echo "========================"

# Check current count in container
CONTAINER_FILES=$(docker exec webhook-processor-prod-webhook-processor-1 find /app -name "raw_webhook_data_*.json" -type f 2>/dev/null | wc -l)
LOCAL_FILES=$(ls debug_files/*.json 2>/dev/null | wc -l)

echo "📊 File Count:"
echo "   Container: $CONTAINER_FILES files"
echo "   Local:     $LOCAL_FILES files"

if [ "$CONTAINER_FILES" -gt "$LOCAL_FILES" ]; then
    echo ""
    echo "🆕 NEW FILES DETECTED!"
    echo "Run this to sync:"
    echo "   docker exec webhook-processor-prod-webhook-processor-1 find /app -name 'raw_webhook_data_*.json' -exec basename {} \; | while read f; do docker cp webhook-processor-prod-webhook-processor-1:/app/\$f debug_files/; done"
elif [ "$CONTAINER_FILES" -eq "$LOCAL_FILES" ]; then
    echo "✅ All files are synced"
else
    echo "⚠️  More local files than container files (unusual)"
fi

echo ""
echo "📋 Recent webhook activity:"
docker logs webhook-processor-prod-webhook-processor-1 | grep "RAW MAILERCLOUD WEBHOOK" | tail -3 | while read line; do
    echo "   📄 $(echo "$line" | jq -r '.timestamp // "Unknown time"')"
done

echo ""
echo "🎯 To monitor continuously: ./monitor_debug_files.sh"
