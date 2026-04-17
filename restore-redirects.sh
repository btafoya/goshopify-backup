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
REDIRECTS_FILE="backups/2026-04-15/url-redirects.json"

ACCESS_TOKEN=""
TOKEN_EXPIRY=0

CREATED=0
UPDATED=0
SKIPPED=0
FAILED=0
CURRENT=0

# --- Validate JSON file ---
if [ ! -f "$REDIRECTS_FILE" ]; then
  echo "❌ Missing $REDIRECTS_FILE"
  exit 1
fi

jq empty "$REDIRECTS_FILE" || { echo "❌ Invalid JSON"; exit 1; }

TOTAL=$(jq length "$REDIRECTS_FILE")

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

# --- Normalize paths ---
normalize_path() {
  local p="$1"
  echo "/${p#/}"
}

echo "🚀 Starting URL redirect restore..."
echo "📊 Total redirects: $TOTAL"
echo ""

# --- Fetch existing redirects ---
echo "🔗 Fetching existing redirects..."
REDIRECTS=$(shopify_api GET "$BASE_URL/redirects.json?limit=250")

# --- Loop ---
jq -c '.[]' "$REDIRECTS_FILE" | while IFS= read -r r; do

  CURRENT=$((CURRENT + 1))

  PATH_VAL=$(echo "$r" | jq -r '.path')
  TARGET_VAL=$(echo "$r" | jq -r '.target')

  PATH_VAL=$(normalize_path "$PATH_VAL")
  TARGET_VAL=$(normalize_path "$TARGET_VAL")

  echo "[$CURRENT/$TOTAL] ➡️ $PATH_VAL → $TARGET_VAL"

  EXISTING=$(safe_jq -c --arg path "$PATH_VAL" \
    '.redirects[]? | select(.path==$path)' <<< "$REDIRECTS")

  REDIRECT_ID=$(echo "$EXISTING" | jq -r '.id // empty' 2>/dev/null || echo "")

  DATA=$(jq -n \
    --arg path "$PATH_VAL" \
    --arg target "$TARGET_VAL" \
    '{ redirect: { path:$path, target:$target } }')

  # --- Diff check ---
  if [ -n "$REDIRECT_ID" ]; then
    CURRENT_TARGET=$(echo "$EXISTING" | jq -r '.target')

    if [ "$CURRENT_TARGET" == "$TARGET_VAL" ]; then
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
  if [ -n "$REDIRECT_ID" ]; then
    echo "   ✏️ Updating"
    shopify_api PUT "$BASE_URL/redirects/$REDIRECT_ID.json" "$DATA" > /dev/null
    UPDATED=$((UPDATED + 1))
  else
    echo "   ➕ Creating"
    RESPONSE=$(shopify_api POST "$BASE_URL/redirects.json" "$DATA")

    NEW_ID=$(echo "$RESPONSE" | jq -r '.redirect.id // empty')

    if [ -z "$NEW_ID" ]; then
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