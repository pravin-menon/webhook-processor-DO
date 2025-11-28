#!/bin/bash

# Test script to check which IP responds to webhook requests
echo "Testing drunkwolf.com IP addresses..."

IPS=("34.93.108.50" "3.33.130.190" "15.197.148.33")

for ip in "${IPS[@]}"; do
    echo "Testing IP: $ip"
    echo "  HTTP Health:"
    curl -s -o /dev/null -w "    Status: %{http_code}, Time: %{time_total}s\n" \
        --connect-timeout 5 "http://$ip/health" 2>/dev/null || echo "    Connection failed"
    
    echo "  HTTP Response Headers:"
    curl -I -s --connect-timeout 5 "http://$ip/" 2>/dev/null | head -3 || echo "    No response"
    
    echo ""
done

echo "Testing domain directly:"
echo "  drunkwolf.com:"
curl -I -s --connect-timeout 5 "http://drunkwolf.com/" 2>/dev/null | head -3 || echo "    No response"

echo ""
echo "If none of these IPs respond with your webhook app, you need to:"
echo "1. Find your actual Google Cloud instance IP"
echo "2. Update DNS to point to your Google Cloud IP"
echo "3. Or use a subdomain like webhooks.drunkwolf.com"
