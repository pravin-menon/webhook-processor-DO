#!/bin/bash

# Test script for webhook validation
# Run this to test different webhook validation scenarios

WEBHOOK_URL="https://drunkwolf.com/webhook"

echo "Testing webhook validation scenarios..."
echo "======================================"

# Test 1: GET request for basic validation
echo "Test 1: GET request validation"
curl -X GET "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -v

echo -e "\n\n"

# Test 2: POST request with MailerCloud User-Agent
echo "Test 2: POST with MailerCloud User-Agent"
curl -X POST "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -H "User-Agent: MailerCloud" \
  -d '{}' \
  -v

echo -e "\n\n"

# Test 3: POST request with WebhookID header
echo "Test 3: POST with WebhookID header"
curl -X POST "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -H "Webhook-Id: WebhookID" \
  -d '{}' \
  -v

echo -e "\n\n"

# Test 4: POST request with test payload
echo "Test 4: POST with test payload"
curl -X POST "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -d '{"test": true}' \
  -v

echo -e "\n\n"

# Test 5: POST request with validation event
echo "Test 5: POST with validation event"
curl -X POST "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -d '{"event": "validation"}' \
  -v

echo -e "\n\n"

# Test 6: Real webhook simulation
echo "Test 6: Real webhook simulation"
curl -X POST "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -H "Webhook-Id: Kyy" \
  -H "Webhook-Type: email.event" \
  -d '{
    "event": "delivered",
    "email": "test@example.com",
    "campaign_id": "12345",
    "ts": 1691234567
  }' \
  -v

echo -e "\n\nAll tests completed!"
