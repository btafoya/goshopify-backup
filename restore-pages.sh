#!/bin/bash

set -e

# --- Load env ---
if [ -f .env ]; then
  export $(grep -v '^#' .env | xargs)
fi

# --- Normalize store URL ---
SHOPIFY_STORE=${SHOPIFY_STORE#https://}
SHOPIFY_STORE=${SHOPIFY_STORE#http://}

# --- Validate ---
if [ -z "$SHOPIFY_STORE" ] || [ -z "$SHOPIFY_CLIENT_ID" ] || [ -z "$SHOPIFY_SECRET" ]; then
  echo "❌ Missing required env vars"
  exit 1
fi

API_VERSION="2025-10"
BASE_URL="https://$SHOPIFY_STORE/admin/api/$API_VERSION"
JSON_FILE="backups/2026-04-15/pages.json"

ACCESS_TOKEN=""
TOKEN_EXPIRY=0

# --- Get Token ---
get_token() {
  echo "🔐 Fetching access token..." >&2

  RESPONSE=$(curl -s --max-time 15 --connect-timeout 5 \
    -X POST "https://$SHOPIFY_STORE/admin/oauth/access_token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=client_credentials" \
    -d "client_id=$SHOPIFY_CLIENT_ID" \
    -d "client_secret=$SHOPIFY_SECRET")

  ACCESS_TOKEN=$(echo "$RESPONSE" | jq -r '.access_token')
  EXPIRES_IN=$(echo "$RESPONSE" | jq -r '.expires_in')

  if [ "$ACCESS_TOKEN" == "null" ] || [ -z "$ACCESS_TOKEN" ]; then
    echo "❌ Failed to obtain token" >&2
    echo "$RESPONSE" >&2
    exit 1
  fi

  TOKEN_EXPIRY=$(( $(date +%s) + EXPIRES_IN - 60 ))

  echo "✅ Token acquired" >&2
}

# --- Ensure Token ---
ensure_token() {
  NOW=$(date +%s)

  if [ -z "$ACCESS_TOKEN" ] || [ "$NOW" -ge "$TOKEN_EXPIRY" ]; then
    get_token
  fi
}

# --- API Wrapper ---
shopify_api() {
  METHOD=$1
  URL=$2
  DATA=$3

  ensure_token

  TMP_BODY=$(mktemp)
  TMP_STATUS=$(mktemp)

  if [ "$METHOD" == "GET" ]; then
    curl -s --max-time 20 --connect-timeout 5 \
      -o "$TMP_BODY" \
      -w "%{http_code}" \
      -X "$METHOD" "$URL" \
      -H "X-Shopify-Access-Token: $ACCESS_TOKEN" \
      -H "Content-Type: application/json" \
      > "$TMP_STATUS"
  else
    curl -s --max-time 20 --connect-timeout 5 \
      -o "$TMP_BODY" \
      -w "%{http_code}" \
      -X "$METHOD" "$URL" \
      -H "X-Shopify-Access-Token: $ACCESS_TOKEN" \
      -H "Content-Type: application/json" \
      -d "$DATA" \
      > "$TMP_STATUS"
  fi

  STATUS=$(cat "$TMP_STATUS")
  BODY=$(cat "$TMP_BODY")

  rm -f "$TMP_BODY" "$TMP_STATUS"

  if [ "$STATUS" == "401" ]; then
    echo "🔁 Token expired, refreshing..." >&2
    get_token
    shopify_api "$METHOD" "$URL" "$DATA"
    return
  fi

  if ! [[ "$STATUS" =~ ^[0-9]+$ ]]; then
    echo "❌ Invalid status response:" >&2
    echo "$STATUS" >&2
    echo "$BODY" >&2
    return 1
  fi

  if [ "$STATUS" -ge 400 ]; then
    echo "❌ API Error ($STATUS)" >&2
    echo "$BODY" >&2
    return 1
  fi

  echo "$BODY"
}

# --- Validate input ---
if [ ! -f "$JSON_FILE" ]; then
  echo "❌ Missing $JSON_FILE"
  exit 1
fi

echo "🚀 Starting restore..."

# --- Fetch existing pages ---
echo "📥 Fetching existing pages..."
EXISTING=$(shopify_api GET "$BASE_URL/pages.json?limit=250" "") || {
  echo "❌ Failed to fetch existing pages"
  exit 1
}
echo "✅ Existing pages fetched"

# --- Process pages ---
while IFS= read -r page; do

  TITLE=$(echo "$page" | jq -r '.title')
  HANDLE=$(echo "$page" | jq -r '.handle')
  BODY=$(echo "$page" | jq -r '.body_html')
  TEMPLATE=$(echo "$page" | jq -r '.template_suffix')

  echo "➡️ Processing: $TITLE ($HANDLE)"

  PAGE_ID=$(echo "$EXISTING" | jq -r ".pages[] | select(.handle==\"$HANDLE\") | .id" | head -n1)

  # --- Build payload safely ---
  if [ -n "$TEMPLATE" ] && [ "$TEMPLATE" != "null" ]; then
    DATA=$(jq -n \
      --arg title "$TITLE" \
      --arg handle "$HANDLE" \
      --arg body "$BODY" \
      --arg template "$TEMPLATE" \
      '{
        page: {
          title: $title,
          handle: $handle,
          body_html: $body,
          template_suffix: $template
        }
      }')
  else
    DATA=$(jq -n \
      --arg title "$TITLE" \
      --arg handle "$HANDLE" \
      --arg body "$BODY" \
      '{
        page: {
          title: $title,
          handle: $handle,
          body_html: $body
        }
      }')
  fi

  if [ -n "$PAGE_ID" ]; then
    echo "✏️ Updating page ID: $PAGE_ID"

    shopify_api PUT \
      "$BASE_URL/pages/$PAGE_ID.json" \
      "$DATA" > /dev/null
  else
    echo "➕ Creating page"

    shopify_api POST \
      "$BASE_URL/pages.json" \
      "$DATA" > /dev/null
  fi

  sleep 0.4

done < <(jq -c '.[]' "$JSON_FILE")

echo "✅ Restore complete."