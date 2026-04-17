#!/bin/bash

set -e
set -o pipefail

trap 'echo "❌ Error on line $LINENO" >&2' ERR

# --- Flags ---
STRICT_MODE=${STRICT_MODE:-0}
DRY_RUN=${DRY_RUN:-0}
RETRY_MODE=${RETRY_MODE:-1}

# --- Load env safely ---
if [ -f .env ]; then
  set -o allexport
  source .env
  set +o allexport
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
JSON_FILE="backups/2026-04-15/collections.json"

ACCESS_TOKEN=""
TOKEN_EXPIRY=0

CREATED=0
UPDATED=0
SKIPPED=0
FAILED=0
CURRENT=0

TOTAL=$(jq length "$JSON_FILE")

# --- Safe jq wrapper ---
safe_jq() {
  jq "$@" 2>/dev/null || true
}

# --- Get Token ---
get_token() {
  echo "🔐 Fetching access token..." >&2

  RESPONSE=$(curl -s \
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

  RESPONSE=$(curl -s \
    -X "$METHOD" "$URL" \
    -H "X-Shopify-Access-Token: $ACCESS_TOKEN" \
    -H "Content-Type: application/json" \
    ${DATA:+-d "$DATA"})

  # Detect API errors
  if echo "$RESPONSE" | jq -e '.errors' > /dev/null 2>&1; then
    echo "⚠️ Shopify API error:" >&2
    echo "$RESPONSE" | jq >&2

    if [ "$STRICT_MODE" -eq 1 ]; then
      exit 1
    fi
  fi

  echo "$RESPONSE"
}

# --- Helpers ---
map_sort_order() {
  case "$1" in
    BEST_SELLING) echo "best-selling" ;;
    CREATED_DESC) echo "created-descending" ;;
    TITLE) echo "alpha-asc" ;;
    TITLE_DESC) echo "alpha-desc" ;;
    PRICE_ASC) echo "price-asc" ;;
    PRICE_DESC) echo "price-desc" ;;
    *) echo "manual" ;;
  esac
}

normalize_rules() {
  echo "$1" | jq 'map({
    column: (if .column=="VARIANT_PRICE" then "variant_price"
             elif .column=="TITLE" then "title"
             elif .column=="TAG" then "tag"
             elif .column=="TYPE" then "type"
             elif .column=="VENDOR" then "vendor"
             else .column end),
    relation: (.relation | ascii_downcase),
    condition: .condition
  })'
}

# --- Validate JSON ---
if [ ! -f "$JSON_FILE" ]; then
  echo "❌ Missing $JSON_FILE"
  exit 1
fi

jq empty "$JSON_FILE" || { echo "❌ Invalid JSON"; exit 1; }

echo "🚀 Starting collection restore..."
echo "📊 Total collections: $TOTAL"
echo ""

CUSTOM=$(shopify_api GET "$BASE_URL/custom_collections.json?limit=250")
SMART=$(shopify_api GET "$BASE_URL/smart_collections.json?limit=250")

# --- Loop ---
jq -c '.[]' "$JSON_FILE" | while IFS= read -r col; do

  CURRENT=$((CURRENT + 1))

  TITLE=$(echo "$col" | jq -r '.title')
  HANDLE=$(echo "$col" | jq -r '.handle')
  BODY=$(echo "$col" | jq -r '.description')
  SORT=$(map_sort_order "$(echo "$col" | jq -r '.sortOrder')")
  RULES_RAW=$(echo "$col" | jq -c '.ruleSet.rules // empty')
  PRODUCTS=$(echo "$col" | jq -c '.products // empty')

  echo "[$CURRENT/$TOTAL] ➡️ $TITLE"

  EXISTING=$(safe_jq -c --arg handle "$HANDLE" \
    '.custom_collections[]? | select(.handle==$handle)' <<< "$CUSTOM")

  if [ -z "$EXISTING" ]; then
    EXISTING=$(safe_jq -c --arg handle "$HANDLE" \
      '.smart_collections[]? | select(.handle==$handle)' <<< "$SMART")
  fi

  COL_ID=$(echo "$EXISTING" | jq -r '.id // empty' 2>/dev/null || echo "")

  # --- Build payload ---
  if [ -n "$RULES_RAW" ] && [ "$RULES_RAW" != "null" ] && [ "$RULES_RAW" != "" ]; then
    TYPE="smart_collections"
    RULES=$(normalize_rules "$RULES_RAW" 2>/dev/null || echo "[]")

    DATA=$(jq -n \
      --arg title "$TITLE" \
      --arg handle "$HANDLE" \
      --arg body "$BODY" \
      --arg sort "$SORT" \
      --argjson rules "$RULES" \
      '{ smart_collection: { title:$title, handle:$handle, body_html:$body, sort_order:$sort, rules:$rules } }')
  else
    TYPE="custom_collections"

    DATA=$(jq -n \
      --arg title "$TITLE" \
      --arg handle "$HANDLE" \
      --arg body "$BODY" \
      --arg sort "$SORT" \
      '{ custom_collection: { title:$title, handle:$handle, body_html:$body, sort_order:$sort } }')
  fi

  # --- Diff ---
  if [ -n "$COL_ID" ]; then
    CURRENT_TITLE=$(echo "$EXISTING" | jq -r '.title')
    CURRENT_BODY=$(echo "$EXISTING" | jq -r '.body_html')

    if [ "$CURRENT_TITLE" == "$TITLE" ] && [ "$CURRENT_BODY" == "$BODY" ]; then
      echo "   ⏭️ Skipped"
      SKIPPED=$((SKIPPED + 1))
      continue
    fi
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    echo "   🧪 Dry run"
    continue
  fi

  # --- Create / Update ---
  if [ -n "$COL_ID" ]; then
    echo "   ✏️ Updating"
    shopify_api PUT "$BASE_URL/$TYPE/$COL_ID.json" "$DATA" > /dev/null
    UPDATED=$((UPDATED + 1))
  else
    echo "   ➕ Creating"
    RESPONSE=$(shopify_api POST "$BASE_URL/$TYPE.json" "$DATA")

    # Retry on handle conflict
    if [ "$RETRY_MODE" -eq 1 ] && echo "$RESPONSE" | jq -e '.errors.handle' > /dev/null 2>&1; then
      echo "   🔁 Retry as update"

      EXISTING=$(safe_jq -c --arg handle "$HANDLE" \
        '.custom_collections[]? | select(.handle==$handle)' <<< "$CUSTOM")

      COL_ID=$(echo "$EXISTING" | jq -r '.id // empty')

      if [ -n "$COL_ID" ]; then
        shopify_api PUT "$BASE_URL/$TYPE/$COL_ID.json" "$DATA" > /dev/null
        ((UPDATED++))
        continue
      fi
    fi

    COL_ID=$(echo "$RESPONSE" | jq -r ".${TYPE%?}.id // empty")

    if [ -z "$COL_ID" ]; then
      echo "   ❌ Failed"
      FAILED=$((FAILED + 1))
      continue
    fi

    CREATED=$((CREATED + 1))
  fi

done

echo ""
echo "✅ Done"
echo "Created: $CREATED"
echo "Updated: $UPDATED"
echo "Skipped: $SKIPPED"
echo "Failed:  $FAILED"