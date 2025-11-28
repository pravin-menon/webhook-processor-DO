#!/bin/bash

# Deployment Test Script for Webhook Processor
# Run this script to test your deployment after DNS is configured

echo "🚀 Testing Webhook Processor Deployment"
echo "======================================="

# Configuration
DOMAIN="drunkwolf.com"
TIMEOUT=10

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to test endpoint
test_endpoint() {
    local url=$1
    local expected_status=$2
    local description=$3
    
    echo -n "Testing $description ($url)... "
    
    response=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout $TIMEOUT "$url" 2>/dev/null)
    
    if [ "$response" = "$expected_status" ]; then
        echo -e "${GREEN}✓ OK (HTTP $response)${NC}"
        return 0
    else
        echo -e "${RED}✗ FAILED (HTTP $response)${NC}"
        return 1
    fi
}

# Function to test webhook validation
test_webhook_validation() {
    local url=$1
    echo -n "Testing webhook validation... "
    
    response=$(curl -s -X POST "$url" \
        -H "Content-Type: application/json" \
        -H "User-Agent: MailerCloud" \
        -d '{}' \
        --connect-timeout $TIMEOUT 2>/dev/null)
    
    if echo "$response" | grep -q '"success":true'; then
        echo -e "${GREEN}✓ OK (Validation successful)${NC}"
        return 0
    else
        echo -e "${RED}✗ FAILED (Validation failed)${NC}"
        echo "Response: $response"
        return 1
    fi
}

echo "📡 Checking DNS resolution..."
if nslookup $DOMAIN > /dev/null 2>&1; then
    IP=$(nslookup $DOMAIN | grep -A1 "Name:" | tail -1 | awk '{print $2}')
    echo -e "${GREEN}✓ DNS resolved: $DOMAIN -> $IP${NC}"
else
    echo -e "${RED}✗ DNS resolution failed for $DOMAIN${NC}"
    echo "Please configure your DNS first (see DNS_CONFIGURATION_GUIDE.md)"
    exit 1
fi

echo ""
echo "🔍 Testing HTTP endpoints..."

# Test basic endpoints
test_endpoint "http://$DOMAIN/health" "200" "Health check (HTTP)"
test_endpoint "http://$DOMAIN/metrics" "200" "Metrics endpoint (HTTP)"

echo ""
echo "🔒 Testing HTTPS endpoints (if SSL is configured)..."

# Test HTTPS endpoints
test_endpoint "https://$DOMAIN/health" "200" "Health check (HTTPS)"
test_endpoint "https://$DOMAIN/metrics" "200" "Metrics endpoint (HTTPS)"

echo ""
echo "📦 Testing webhook validation..."

# Test webhook validation (HTTP and HTTPS)
test_webhook_validation "http://$DOMAIN/webhook"
test_webhook_validation "https://$DOMAIN/webhook"

echo ""
echo "📊 Testing monitoring endpoints (if enabled)..."

# Test monitoring endpoints
test_endpoint "http://$DOMAIN:9091" "200" "Prometheus (HTTP)"
test_endpoint "http://$DOMAIN/grafana/" "200" "Grafana (HTTP)"

echo ""
echo "🐳 Checking Docker services..."

# Check if running in Docker context
if command -v docker-compose &> /dev/null; then
    if [ -f "docker-compose.prod.yml" ]; then
        echo "Docker services status:"
        docker-compose -f docker-compose.prod.yml ps
        
        echo ""
        echo "Recent logs from webhook-processor:"
        docker-compose -f docker-compose.prod.yml logs --tail=5 webhook-processor
    else
        echo -e "${YELLOW}⚠ docker-compose.prod.yml not found in current directory${NC}"
    fi
else
    echo -e "${YELLOW}⚠ docker-compose not available${NC}"
fi

echo ""
echo "✅ Testing completed!"
echo ""
echo "📋 Summary:"
echo "- Health endpoint: https://$DOMAIN/health"
echo "- Webhook endpoint: https://$DOMAIN/webhook"
echo "- Metrics endpoint: https://$DOMAIN/metrics"
echo "- Grafana dashboard: https://$DOMAIN/grafana/ (if monitoring enabled)"
echo "- Prometheus: https://$DOMAIN:9091 (if monitoring enabled)"
echo ""
echo "💡 To enable monitoring services:"
echo "   docker-compose -f docker-compose.prod.yml --profile monitoring up -d"
