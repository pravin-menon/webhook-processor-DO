#!/bin/bash

echo "🔎 Webhook Debug Analysis Report"
echo "================================="
echo ""

# Function to analyze a JSON file
analyze_file() {
    local file="$1"
    local num="$2"
    
    echo "📄 File $num: $(basename "$file")"
    echo "─────────────────────────────────────"
    
    # Extract key information
    echo "🕒 Timestamp: $(jq -r '.timestamp' "$file")"
    echo "🌐 User Agent: $(jq -r '.user_agent' "$file")"
    echo "📡 Remote IP: $(jq -r '.remote_ip' "$file")"
    echo ""
    
    # API Key analysis
    echo "🔑 API Key Analysis:"
    api_key=$(jq -r '.headers["X-Api-Key"][0]' "$file")
    if [ "$api_key" != "null" ]; then
        echo "  API Key: ${api_key:0:20}...${api_key: -10}"
        # You can add logic here to map API keys to clients
        case "$api_key" in
            "saAud-6cb5f951c03befd6699b57de67ba88c9"*)
                echo "  → Client: MailerCloud Client 1"
                ;;
            "CAAEI-39a346d00bea7941ccbe84532b4e5b2b"*)
                echo "  → Client: MailerCloud Client 2"
                ;;
            *)
                echo "  → Client: Unknown"
                ;;
        esac
    else
        echo "  No API key found"
    fi
    echo ""
    
    # Body analysis
    echo "📝 Payload Analysis:"
    echo "  Event: $(jq -r '.body.event // "N/A"' "$file")"
    echo "  Email: $(jq -r '.body.email // "N/A"' "$file")"
    echo "  Campaign ID: $(jq -r '.body.campaign_id // .body.camp_id // "N/A"' "$file")"
    echo "  Campaign Name: $(jq -r '.body.campaign_name // "N/A"' "$file")"
    echo ""
    
    # Domain extraction
    email=$(jq -r '.body.email // "N/A"' "$file")
    if [ "$email" != "N/A" ] && [ "$email" != "null" ]; then
        domain=$(echo "$email" | cut -d'@' -f2)
        echo "  📧 Email Domain: $domain"
        echo "  🏢 Potential Client ID: domain_$domain"
    fi
    echo ""
    
    # All fields present
    echo "📋 All Available Fields:"
    jq -r '.body | keys[]' "$file" | sort | sed 's/^/  - /'
    echo ""
    
    # Client identification possibilities
    echo "🎯 Client Identification Strategies:"
    
    # Strategy 1: API Key mapping (most reliable)
    if [ "$api_key" != "null" ]; then
        echo "  ✅ Strategy 1: API Key mapping (RECOMMENDED)"
        echo "     Reliability: HIGH"
    fi
    
    # Strategy 2: Email domain
    if [ "$email" != "N/A" ] && [ "$email" != "null" ]; then
        echo "  ✅ Strategy 2: Email domain extraction"
        echo "     Reliability: MEDIUM (depends on client email patterns)"
    fi
    
    # Strategy 3: Look for client-specific fields
    client_fields=$(jq -r '.body | keys[]' "$file" | grep -E 'client|customer|account|tenant|sender' || true)
    if [ ! -z "$client_fields" ]; then
        echo "  ✅ Strategy 3: Client-specific fields found:"
        echo "$client_fields" | sed 's/^/     - /'
        echo "     Reliability: HIGH (if consistent)"
    else
        echo "  ❌ Strategy 3: No client-specific fields found"
    fi
    
    # Strategy 4: Campaign pattern analysis
    campaign_id=$(jq -r '.body.campaign_id // .body.camp_id // "N/A"' "$file")
    if [ "$campaign_id" != "N/A" ] && [ "$campaign_id" != "null" ]; then
        echo "  ✅ Strategy 4: Campaign ID pattern analysis"
        echo "     Campaign ID: $campaign_id"
        echo "     Reliability: LOW (campaign IDs may not be client-specific)"
    fi
    
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo ""
}

# Analyze all debug files
file_count=1
for file in debug_files/*.json; do
    if [ -f "$file" ]; then
        analyze_file "$file" "$file_count"
        ((file_count++))
    fi
done

# Summary and recommendations
echo "📊 SUMMARY & RECOMMENDATIONS"
echo "=============================="
echo ""
echo "🎯 Recommended Client Identification Strategy:"
echo ""
echo "1. 🥇 PRIMARY: API Key Mapping"
echo "   - Map X-API-Key header values to client IDs"
echo "   - Most reliable and secure method"
echo "   - Already implemented in your current system"
echo ""
echo "2. 🥈 FALLBACK: Email Domain Extraction"
echo "   - Extract domain from email field"
echo "   - Useful for domain-based client separation"
echo "   - Currently being used as backup strategy"
echo ""
echo "3. 🥉 BACKUP: Look for client-specific fields in future webhooks"
echo "   - Monitor for fields like client_id, account_id, etc."
echo "   - Update strategy if MailerCloud adds such fields"
echo ""
echo "📝 Next Steps:"
echo "1. Keep debug mode running to capture real MailerCloud webhooks"
echo "2. Analyze patterns in actual webhook data"
echo "3. Consider switching to production mode once satisfied with client identification"
echo "4. Monitor the generated webhook_id patterns for uniqueness"
echo ""
echo "🚀 Your current setup is working well with API key-based client identification!"
